import { randomUUID } from "node:crypto";

export class ApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string,
    message: string,
    public readonly requestId?: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

export interface Me {
  scopes: string[];
  raw: unknown;
}

export interface RequestOptions {
  query?: Record<string, string | number | boolean | undefined>;
  body?: unknown;
}

export interface ClientOptions {
  baseUrl: string;
  apiKey: string;
  timeoutMs: number;
  /** Propagated as X-Request-Id; generated when omitted. */
  requestId?: string;
  fetchImpl?: typeof fetch;
}

/** Typed client for the SocialOS REST API, authenticated with the caller's API key. */
export class SocialOSClient {
  readonly requestId: string;
  private readonly fetchImpl: typeof fetch;

  constructor(private readonly opts: ClientOptions) {
    this.requestId = opts.requestId ?? randomUUID();
    this.fetchImpl = opts.fetchImpl ?? fetch;
  }

  async request<T = unknown>(method: string, path: string, o: RequestOptions = {}): Promise<T> {
    const url = new URL(this.opts.baseUrl + path);
    for (const [k, v] of Object.entries(o.query ?? {})) {
      if (v !== undefined) url.searchParams.set(k, String(v));
    }
    const headers: Record<string, string> = {
      Authorization: `Bearer ${this.opts.apiKey}`,
      Accept: "application/json",
      "X-Request-Id": this.requestId,
      "User-Agent": "socialos-mcp/0.1",
    };
    let body: string | undefined;
    if (o.body !== undefined) {
      headers["Content-Type"] = "application/json";
      body = JSON.stringify(o.body);
    }

    let res: Response;
    try {
      res = await this.fetchImpl(url, { method, headers, body, signal: AbortSignal.timeout(this.opts.timeoutMs) });
    } catch (err) {
      const timedOut = err instanceof Error && (err.name === "TimeoutError" || err.name === "AbortError");
      throw new ApiError(
        timedOut ? 504 : 503,
        timedOut ? "UPSTREAM_TIMEOUT" : "UPSTREAM_UNAVAILABLE",
        timedOut ? "The SocialOS API timed out" : "The SocialOS API is unreachable",
        this.requestId,
      );
    }

    const text = await res.text();
    let json: unknown = undefined;
    if (text) {
      try {
        json = JSON.parse(text);
      } catch {
        json = undefined;
      }
    }
    if (!res.ok) {
      const e = (json as { error?: { code?: string; message?: string; request_id?: string } } | undefined)?.error;
      throw new ApiError(
        res.status,
        e?.code ?? defaultCode(res.status),
        e?.message ?? `SocialOS API returned HTTP ${res.status}`,
        e?.request_id ?? res.headers.get("x-request-id") ?? this.requestId,
      );
    }
    return (json ?? { ok: true }) as T;
  }

  /** GET /me → the key's scopes. Accepts `scopes` at top level or nested under api_key / user. */
  async me(): Promise<Me> {
    const raw = await this.request<Record<string, unknown>>("GET", "/me");
    const nested = (k: string): unknown => (raw[k] as Record<string, unknown> | undefined)?.scopes;
    const candidate = raw.scopes ?? nested("api_key") ?? nested("key") ?? nested("user") ?? nested("auth");
    const scopes = Array.isArray(candidate) ? candidate.filter((s): s is string => typeof s === "string") : [];
    return { scopes, raw };
  }
}

function defaultCode(status: number): string {
  switch (status) {
    case 400: return "VALIDATION_ERROR";
    case 401: return "UNAUTHENTICATED";
    case 403: return "FORBIDDEN";
    case 404: return "NOT_FOUND";
    case 409: return "CONFLICT";
    case 429: return "RATE_LIMITED";
    default: return status >= 500 ? "INTERNAL" : "ERROR";
  }
}
