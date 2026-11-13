import type { ApiResponse, Nullable, SqlNull } from "../types/api";
import { RATE_LIMIT_MESSAGE, createLimiter, createPromiseCache, retryOnStatus, withSignal, type RawResult } from "./resilience";

export { RATE_LIMIT_MESSAGE, isRateLimitText } from "./resilience";

const API_BASE = (import.meta.env.VITE_API_BASE_URL ?? "/api/v1").replace(/\/+$/, "");

export { API_BASE };

export type QueryParams = Record<string, string | number | boolean | undefined | null>;

export class ApiError extends Error {
  status: number;
  constructor(message: string, status: number) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

export interface RequestOptions {
  // Abort waiting (also backoff/queue waits). A request shared through the
  // geometry cache keeps running for other callers.
  signal?: AbortSignal;
}

// Server side (Traefik) rate-limits expensive GETs per IP (4 concurrent, 20/min).
// Geometry requests are therefore limited to 2 concurrent and cached/deduplicated
// per URL for this session (geometry changes at most hourly; failures are not cached).
const geometryLimiter = createLimiter(2);
const geometryCache = createPromiseCache<ApiResponse<unknown>>(20, 30 * 60_000);

// POST endpoints that are pure queries (no writes) and thus safe to repeat.
const IDEMPOTENT_POST_PATHS = new Set(["/custom-regions/dataset-aggregate", "/geo/query"]);

function isGeoWithGeometry(path: string, params?: QueryParams): boolean {
  return path === "/geo" && (params?.include_geometry === true || params?.include_geometry === "true");
}

// Heavy requests share the concurrency limiter (matches the server-side "heavy" routes).
function isGeometryRequest(path: string, params?: QueryParams): boolean {
  return path.startsWith("/regions/") || isGeoWithGeometry(path, params);
}

// Only boundary geometries are cached for the session: /regions/.../data returns values that change
// with the hourly pipeline run and must not be served stale.
function isCacheableRequest(path: string, params?: QueryParams): boolean {
  return isGeoWithGeometry(path, params);
}

async function fetchRaw(url: string, init?: RequestInit): Promise<RawResult> {
  const res = await fetch(url, init);
  return { status: res.status, retryAfter: res.headers.get("Retry-After"), text: await res.text() };
}

function parseResponse<T>(raw: RawResult): ApiResponse<T> {
  if (raw.status === 429) throw new ApiError(RATE_LIMIT_MESSAGE, 429);
  let body: ApiResponse<T> | undefined;
  try {
    body = JSON.parse(raw.text);
  } catch {
    throw new ApiError(`Ungültige Antwort (HTTP ${raw.status})`, raw.status);
  }
  if (raw.status < 200 || raw.status >= 300 || !body || body.success === false) {
    throw new ApiError(body?.error || `HTTP ${raw.status}`, raw.status);
  }
  return body;
}

export async function apiGet<T>(path: string, params?: QueryParams, opts: RequestOptions = {}): Promise<ApiResponse<T>> {
  let url = API_BASE + path;
  if (params) {
    const query = Object.entries(params)
      .filter(([, value]) => value !== undefined && value !== null && value !== "")
      .map(([key, value]) => `${encodeURIComponent(key)}=${encodeURIComponent(String(value))}`)
      .join("&");
    if (query) url += "?" + query;
  }

  const { signal } = opts;
  if (isCacheableRequest(path, params)) {
    // No signal on the shared request itself: other callers may still need it.
    const shared = geometryCache.get(url, async () =>
      parseResponse(await retryOnStatus(() => geometryLimiter(() => fetchRaw(url)))),
    );
    return withSignal(shared as Promise<ApiResponse<T>>, signal);
  }
  if (isGeometryRequest(path, params)) {
    return parseResponse<T>(
      await retryOnStatus(() => geometryLimiter(() => fetchRaw(url, { signal })), { signal }),
    );
  }
  return parseResponse<T>(await retryOnStatus(() => fetchRaw(url, { signal }), { signal }));
}

export async function apiPost<T>(path: string, body: unknown, opts: RequestOptions = {}): Promise<ApiResponse<T>> {
  const { signal } = opts;
  const attempt = () =>
    fetchRaw(API_BASE + path, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
      signal,
    });
  // Non-idempotent POSTs get exactly one attempt.
  const raw = IDEMPOTENT_POST_PATHS.has(path) ? await retryOnStatus(attempt, { signal }) : await attempt();
  return parseResponse<T>(raw);
}

// The Go API serializes some nullable fields (LAU metadata) as
// database/sql.Null* structs ({"Int64":1,"Valid":true}) instead of plain
// scalars. Unwrap both forms so the UI doesn't depend on the shape.
export function unwrapNull<T>(field: Nullable<T>): T | null {
  if (field && typeof field === "object" && "Valid" in field) {
    const wrapped = field as SqlNull;
    if (!wrapped.Valid) return null;
    if ("Int64" in wrapped) return wrapped.Int64 as unknown as T;
    if ("String" in wrapped) return wrapped.String as unknown as T;
    if ("Bool" in wrapped) return wrapped.Bool as unknown as T;
    if ("Float64" in wrapped) return wrapped.Float64 as unknown as T;
    return null;
  }
  return (field ?? null) as T | null;
}
