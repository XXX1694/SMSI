/** The HTTP core of the API client: base URLs, CSRF, the request function. Only api.ts imports it (ESLint enforces that). */
import { ApiError, parseErrorBody } from './api-error';
import { enErrorsT } from '@/i18n/en';
import { describeErrorCode } from './errors';

export const API_BASE = '/api/v1';
/**
 * Where uploads go. The Next.js rewrite proxy buffers and truncates request bodies at 10 MB, so large uploads
 * (videos, up to 100 MB) go straight to the API host (NEXT_PUBLIC_API_URL, baked at build time; CORS and the session
 * cookie, set for the parent domain, already allow it, see D-015). Without it (`next dev`) they use the proxy.
 */
export function uploadBase(): string {
  const direct = (process.env.NEXT_PUBLIC_API_URL ?? '').replace(/\/+$/, '');
  return direct ? `${direct}/api/v1` : API_BASE;
}


let csrfToken: string | null = null;
export function setCsrfToken(token: string | null): void {
  csrfToken = token;
}

function readCsrfCookie(): string | null {
  if (typeof document === 'undefined') return null;
  const m = /(?:^|;\s*)socialos_csrf=([^;]+)/.exec(document.cookie);
  return m?.[1] ? decodeURIComponent(m[1]) : null;
}

type Query = Record<string, string | number | undefined | null>;

interface RequestOptions {
  method?: 'GET' | 'POST' | 'PATCH' | 'DELETE';
  query?: Query;
  body?: unknown;
  form?: FormData;
  /** Send straight to the API host instead of through the same-origin proxy (see uploadBase). */
  direct?: boolean;
}

export function buildQuery(query?: Query): string {
  if (!query) return '';
  const p = new URLSearchParams();
  for (const [k, v] of Object.entries(query)) {
    if (v !== undefined && v !== null && v !== '') p.set(k, String(v));
  }
  const s = p.toString();
  return s ? `?${s}` : '';
}

export async function request(path: string, opts: RequestOptions = {}): Promise<unknown> {
  const method = opts.method ?? 'GET';
  // Demo build only (`npm run build:demo`): answer from the in-browser mock instead of the network.
  // The condition is a literal so the normal build drops this branch and never ships the mock.
  if (process.env.NEXT_PUBLIC_DEMO === 'true') {
    const { demoFetch } = await import('./demo');
    const res = await demoFetch({ method, path, query: opts.query, body: opts.body, form: opts.form });
    if (res.status >= 400) throw parseErrorBody(res.status, res.body);
    return res.body ?? null;
  }
  const headers: Record<string, string> = { Accept: 'application/json' };
  let body: BodyInit | undefined;
  if (opts.form) {
    body = opts.form;
  } else if (opts.body !== undefined) {
    headers['Content-Type'] = 'application/json';
    body = JSON.stringify(opts.body);
  }
  if (method !== 'GET') {
    const token = csrfToken ?? readCsrfCookie();
    if (token) headers['X-CSRF-Token'] = token;
  }
  let res: Response;
  try {
    const base = opts.direct ? uploadBase() : API_BASE;
    res = await fetch(`${base}${path}${buildQuery(opts.query)}`, {
      method,
      headers,
      body,
      credentials: opts.direct && base !== API_BASE ? 'include' : 'same-origin',
    });
  } catch {
    throw new ApiError(0, 'NETWORK', describeErrorCode('NETWORK', enErrorsT));
  }
  const text = await res.text();
  let json: unknown = null;
  if (text) {
    try {
      json = JSON.parse(text);
    } catch {
      json = null;
    }
  }
  if (!res.ok) throw parseErrorBody(res.status, json);
  return json;
}

export const enc = encodeURIComponent;
