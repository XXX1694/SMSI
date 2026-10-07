import {
  buildMcpConfig,
  normalizeCreatedApiKey,
  normalizeCreatedMcp,
  normalizePage,
  normalizeProvider,
  unwrapList,
} from './normalize';
import type {
  AnalyticsResult,
  ApiKey,
  AuditLog,
  CreatePostInput,
  CreatedApiKey,
  CreatedMcpConnection,
  DashboardSummary,
  Me,
  McpConnection,
  Media,
  Page,
  Post,
  Provider,
  SocialAccount,
  UsageSummary,
} from './types';

export const API_BASE = '/api/v1';
const MCP_URL = process.env.NEXT_PUBLIC_MCP_URL ?? 'http://localhost:3333/mcp';
const PUBLIC_API_URL = process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8080';

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly requestId: string | null;

  constructor(status: number, code: string, message: string, requestId: string | null = null) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
    this.requestId = requestId;
  }
}

/** Parse the uniform `{"error":{code,message,request_id}}` format; tolerate anything else. */
export function parseErrorBody(status: number, body: unknown): ApiError {
  if (typeof body === 'object' && body !== null && 'error' in body) {
    const e = (body as { error: unknown }).error;
    if (typeof e === 'object' && e !== null) {
      const r = e as Record<string, unknown>;
      return new ApiError(
        status,
        typeof r.code === 'string' ? r.code : 'UNKNOWN',
        typeof r.message === 'string' ? r.message : `Request failed (${status})`,
        typeof r.request_id === 'string' ? r.request_id : null,
      );
    }
  }
  const fallback: Record<number, [string, string]> = {
    401: ['UNAUTHENTICATED', 'Please sign in.'],
    403: ['FORBIDDEN', 'You are not allowed to do that.'],
    404: ['NOT_FOUND', 'Not found.'],
    429: ['RATE_LIMITED', 'Too many requests. Try again shortly.'],
  };
  const [code, message] = fallback[status] ?? [status >= 500 ? 'INTERNAL' : 'UNKNOWN', `Request failed (${status}).`];
  return new ApiError(status, code, message);
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

async function request(path: string, opts: RequestOptions = {}): Promise<unknown> {
  const method = opts.method ?? 'GET';
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
    res = await fetch(`${API_BASE}${path}${buildQuery(opts.query)}`, {
      method,
      headers,
      body,
      credentials: 'same-origin',
    });
  } catch {
    throw new ApiError(0, 'NETWORK', 'Cannot reach the server. Check your connection.');
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

const enc = encodeURIComponent;

export interface PostFilters {
  status?: string;
  from?: string;
  to?: string;
  limit?: number;
  cursor?: string;
}

export const api = {
  auth: {
    async register(input: { email: string; password: string; display_name: string }): Promise<Me> {
      const me = (await request('/auth/register', { method: 'POST', body: input })) as Me;
      return me;
    },
    async login(input: { email: string; password: string }): Promise<Me> {
      return (await request('/auth/login', { method: 'POST', body: input })) as Me;
    },
    async logout(): Promise<void> {
      await request('/auth/logout', { method: 'POST' });
    },
    async me(): Promise<Me> {
      return (await request('/me')) as Me;
    },
  },
  social: {
    async providers(): Promise<Provider[]> {
      return unwrapList<unknown>(await request('/social/providers')).map(normalizeProvider);
    },
    async accounts(): Promise<SocialAccount[]> {
      return unwrapList<SocialAccount>(await request('/social/accounts'));
    },
    connectUrl(provider: string): string {
      return `${API_BASE}/social/${enc(provider)}/connect?redirect=${enc('/accounts')}`;
    },
    async connectTelegram(chat: string): Promise<SocialAccount> {
      return (await request('/social/telegram/connect', { method: 'POST', body: { chat } })) as SocialAccount;
    },
    async disconnect(id: string): Promise<void> {
      await request(`/social/accounts/${enc(id)}`, { method: 'DELETE' });
    },
  },
  posts: {
    async list(f: PostFilters = {}): Promise<Page<Post>> {
      return normalizePage<Post>(await request('/posts', { query: { ...f } }));
    },
    async get(id: string): Promise<Post> {
      return (await request(`/posts/${enc(id)}`)) as Post;
    },
    async create(input: CreatePostInput): Promise<Post> {
      return (await request('/posts', { method: 'POST', body: input })) as Post;
    },
    async publish(id: string): Promise<Post> {
      return (await request(`/posts/${enc(id)}/publish`, { method: 'POST' })) as Post;
    },
    async schedule(id: string, scheduledAt: string): Promise<Post> {
      return (await request(`/posts/${enc(id)}/schedule`, { method: 'POST', body: { scheduled_at: scheduledAt } })) as Post;
    },
    async cancel(id: string): Promise<Post> {
      return (await request(`/posts/${enc(id)}/cancel`, { method: 'POST' })) as Post;
    },
    async retry(id: string): Promise<Post> {
      return (await request(`/posts/${enc(id)}/retry`, { method: 'POST' })) as Post;
    },
    async remove(id: string): Promise<void> {
      await request(`/posts/${enc(id)}`, { method: 'DELETE' });
    },
  },
  media: {
    async list(): Promise<Media[]> {
      return unwrapList<Media>(await request('/media'));
    },
    async upload(file: File): Promise<Media> {
      const form = new FormData();
      form.append('file', file);
      return (await request('/media', { method: 'POST', form })) as Media;
    },
    async get(id: string): Promise<Media> {
      return (await request(`/media/${enc(id)}`)) as Media;
    },
    async remove(id: string): Promise<void> {
      await request(`/media/${enc(id)}`, { method: 'DELETE' });
    },
  },
  dashboard: {
    async summary(): Promise<DashboardSummary> {
      const r = (await request('/dashboard/summary')) as Partial<DashboardSummary>;
      return {
        connected_accounts: r.connected_accounts ?? 0,
        scheduled_posts: r.scheduled_posts ?? 0,
        drafts: r.drafts ?? 0,
        published_this_month: r.published_this_month ?? 0,
        failed: r.failed ?? 0,
        upcoming: r.upcoming ?? [],
        recent: r.recent ?? [],
      };
    },
  },
  analytics: {
    async get(from?: string, to?: string): Promise<AnalyticsResult> {
      return { items: unwrapList(await request('/analytics', { query: { from, to } })) };
    },
  },
  audit: {
    async list(limit = 50, cursor?: string): Promise<Page<AuditLog>> {
      return normalizePage<AuditLog>(await request('/audit-logs', { query: { limit, cursor } }));
    },
  },
  developer: {
    async apiKeys(): Promise<ApiKey[]> {
      return unwrapList<ApiKey>(await request('/developer/api-keys'));
    },
    async createApiKey(input: { name: string; scopes: string[]; expires_at?: string }): Promise<CreatedApiKey> {
      return normalizeCreatedApiKey(await request('/developer/api-keys', { method: 'POST', body: input }));
    },
    async revokeApiKey(id: string): Promise<void> {
      await request(`/developer/api-keys/${enc(id)}`, { method: 'DELETE' });
    },
    async mcpConnections(): Promise<McpConnection[]> {
      return unwrapList<McpConnection>(await request('/developer/mcp-connections'));
    },
    async createMcpConnection(input: { name: string; scopes: string[] }): Promise<CreatedMcpConnection> {
      const raw = await request('/developer/mcp-connections', { method: 'POST', body: input });
      return normalizeCreatedMcp(raw, MCP_URL, PUBLIC_API_URL);
    },
    async revokeMcpConnection(id: string): Promise<void> {
      await request(`/developer/mcp-connections/${enc(id)}`, { method: 'DELETE' });
    },
    async usage(): Promise<UsageSummary> {
      const r = (await request('/developer/usage')) as Partial<UsageSummary>;
      return { total_requests: r.total_requests ?? 0, by_key: r.by_key ?? [], by_day: r.by_day ?? [] };
    },
  },
};

export { buildMcpConfig };
