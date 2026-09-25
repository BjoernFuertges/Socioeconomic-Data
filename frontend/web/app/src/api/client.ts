import type { ApiResponse, Nullable, SqlNull } from "../types/api";

const API_BASE = (import.meta.env.VITE_API_BASE_URL ?? "/api/v1").replace(/\/+$/, "");

export { API_BASE };

export type QueryParams = Record<string, string | number | boolean | undefined | null>;

export async function apiGet<T>(path: string, params?: QueryParams): Promise<ApiResponse<T>> {
  let url = API_BASE + path;
  if (params) {
    const query = Object.entries(params)
      .filter(([, value]) => value !== undefined && value !== null && value !== "")
      .map(([key, value]) => `${encodeURIComponent(key)}=${encodeURIComponent(String(value))}`)
      .join("&");
    if (query) url += "?" + query;
  }

  const res = await fetch(url);
  let body: ApiResponse<T> | undefined;
  try {
    body = await res.json();
  } catch {
    throw new Error(`Ungültige Antwort (HTTP ${res.status})`);
  }
  if (!res.ok || !body || body.success === false) {
    throw new Error(body?.error || `HTTP ${res.status}`);
  }
  return body;
}

export async function apiPost<T>(path: string, body: unknown): Promise<ApiResponse<T>> {
  const res = await fetch(API_BASE + path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  let parsed: ApiResponse<T> | undefined;
  try {
    parsed = await res.json();
  } catch {
    throw new Error(`Ungültige Antwort (HTTP ${res.status})`);
  }
  if (!res.ok || !parsed || parsed.success === false) {
    throw new Error(parsed?.error || `HTTP ${res.status}`);
  }
  return parsed;
}

// Die Go-API serialisiert einige nullable Felder (LAU-Metadaten) als
// database/sql.Null*-Structs ({"Int64":1,"Valid":true}) statt schlichter
// Skalare. Beide Formen entpacken, damit die UI unabhängig vom Shape läuft.
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
