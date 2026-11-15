// Small, framework-free building blocks for the API client: retry with backoff
// for rate limiting (HTTP 429/503), a concurrency limiter and a bounded promise
// cache. Kept free of fetch/DOM/Vite specifics so they can be tested with plain
// Node and mocks.

export const RATE_LIMIT_MESSAGE = "Zu viele Anfragen - bitte einen Moment warten und erneut versuchen";

export function isRateLimitText(text: string | null | undefined): boolean {
  return !!text && text.includes(RATE_LIMIT_MESSAGE);
}

export interface RawResult {
  status: number;
  retryAfter: string | null;
  text: string;
}

export interface RetryOptions {
  maxAttempts?: number; // total attempts including the first one
  baseDelayMs?: number;
  maxDelayMs?: number;
  retryStatuses?: readonly number[];
  signal?: AbortSignal;
  sleep?: (ms: number, signal?: AbortSignal) => Promise<void>;
  random?: () => number;
}

export function abortError(): Error {
  return new DOMException("Aborted", "AbortError");
}

export function sleepAbortable(ms: number, signal?: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    if (signal?.aborted) return reject(abortError());
    const onAbort = () => {
      clearTimeout(timer);
      reject(abortError());
    };
    const timer = setTimeout(() => {
      signal?.removeEventListener("abort", onAbort);
      resolve();
    }, ms);
    signal?.addEventListener("abort", onAbort, { once: true });
  });
}

// Retry-After is either delta-seconds or an HTTP date. Returns ms or null.
export function parseRetryAfter(value: string | null, now = Date.now()): number | null {
  if (!value) return null;
  const secs = Number(value);
  if (Number.isFinite(secs)) return Math.max(0, secs * 1000);
  const date = Date.parse(value);
  return Number.isNaN(date) ? null : Math.max(0, date - now);
}

// Runs `attempt` and repeats it while the response status is in retryStatuses
// (default 429/503): exponential backoff (base*2^n) with jitter, Retry-After
// wins if larger, every delay capped at maxDelayMs. Returns the last result
// (also if it is still 429) - the caller turns it into an error. Network
// errors/aborts are thrown immediately, never retried.
export async function retryOnStatus(attempt: () => Promise<RawResult>, opts: RetryOptions = {}): Promise<RawResult> {
  const maxAttempts = opts.maxAttempts ?? 3;
  const base = opts.baseDelayMs ?? 1000;
  const cap = opts.maxDelayMs ?? 10_000;
  const statuses = opts.retryStatuses ?? [429, 503];
  const sleep = opts.sleep ?? sleepAbortable;
  const random = opts.random ?? Math.random;
  let res: RawResult;
  for (let n = 1; ; n++) {
    if (opts.signal?.aborted) throw abortError();
    res = await attempt();
    if (!statuses.includes(res.status) || n >= maxAttempts) return res;
    const backoff = base * 2 ** (n - 1) * (0.5 + random() * 0.5);
    const delay = Math.min(cap, Math.max(backoff, parseRetryAfter(res.retryAfter) ?? 0));
    await sleep(delay, opts.signal);
  }
}

// FIFO concurrency limiter: at most `max` tasks run at once. A finished task
// hands its slot directly to the next waiter.
export function createLimiter(max: number) {
  let active = 0;
  const waiting: (() => void)[] = [];
  const release = () => {
    const nextWaiter = waiting.shift();
    if (nextWaiter) nextWaiter();
    else active--;
  };
  return async function limit<T>(task: () => Promise<T>, signal?: AbortSignal): Promise<T> {
    if (signal?.aborted) throw abortError();
    if (active < max) {
      active++;
    } else {
      await new Promise<void>((resolve, reject) => {
        const entry = () => {
          signal?.removeEventListener("abort", onAbort);
          resolve();
        };
        const onAbort = () => {
          const i = waiting.indexOf(entry);
          if (i >= 0) waiting.splice(i, 1);
          reject(abortError());
        };
        signal?.addEventListener("abort", onAbort, { once: true });
        waiting.push(entry);
      });
    }
    try {
      return await task();
    } finally {
      release();
    }
  };
}

// LRU promise cache with TTL. Failed promises are removed again, so errors are
// never served from the cache; concurrent identical calls share one promise.
export function createPromiseCache<T>(maxEntries = 20, ttlMs = 30 * 60_000, now: () => number = Date.now) {
  const map = new Map<string, { promise: Promise<T>; at: number }>();
  return {
    get(key: string, factory: () => Promise<T>): Promise<T> {
      const hit = map.get(key);
      if (hit && now() - hit.at < ttlMs) {
        map.delete(key);
        map.set(key, hit); // refresh LRU position
        return hit.promise;
      }
      map.delete(key);
      const promise = factory();
      const entry = { promise, at: now() };
      map.set(key, entry);
      promise.catch(() => {
        if (map.get(key) === entry) map.delete(key);
      });
      while (map.size > maxEntries) map.delete(map.keys().next().value as string);
      return promise;
    },
    clear: () => map.clear(),
    get size() {
      return map.size;
    },
  };
}

// Lets one caller stop waiting without cancelling a shared underlying request.
export function withSignal<T>(promise: Promise<T>, signal?: AbortSignal): Promise<T> {
  if (!signal) return promise;
  return new Promise<T>((resolve, reject) => {
    if (signal.aborted) return reject(abortError());
    const onAbort = () => reject(abortError());
    signal.addEventListener("abort", onAbort, { once: true });
    promise.then(
      (v) => {
        signal.removeEventListener("abort", onAbort);
        resolve(v);
      },
      (e) => {
        signal.removeEventListener("abort", onAbort);
        reject(e);
      },
    );
  });
}
