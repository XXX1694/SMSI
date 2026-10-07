import { afterEach, describe, expect, it, vi } from 'vitest';
import { api, ApiError, buildQuery, parseErrorBody, setCsrfToken } from '@/lib/api';

function mockFetch(status: number, body: unknown) {
  const fn = vi.fn(async () => new Response(body === undefined ? null : JSON.stringify(body), { status }));
  vi.stubGlobal('fetch', fn);
  return fn;
}

afterEach(() => {
  vi.unstubAllGlobals();
  setCsrfToken(null);
});

describe('parseErrorBody', () => {
  it('parses the uniform error format', () => {
    const e = parseErrorBody(422, { error: { code: 'SOCIAL_ACCOUNT_EXPIRED', message: 'LinkedIn authorization has expired', request_id: 'abc' } });
    expect(e).toBeInstanceOf(ApiError);
    expect(e.status).toBe(422);
    expect(e.code).toBe('SOCIAL_ACCOUNT_EXPIRED');
    expect(e.message).toBe('LinkedIn authorization has expired');
    expect(e.requestId).toBe('abc');
  });

  it('falls back sensibly for non-JSON bodies', () => {
    expect(parseErrorBody(401, null).code).toBe('UNAUTHENTICATED');
    expect(parseErrorBody(502, 'Bad gateway').code).toBe('INTERNAL');
    expect(parseErrorBody(418, {}).code).toBe('UNKNOWN');
  });

  it('tolerates a malformed error object', () => {
    const e = parseErrorBody(400, { error: { code: 5 } });
    expect(e.code).toBe('UNKNOWN');
    expect(e.message).toContain('400');
  });
});

describe('buildQuery', () => {
  it('skips empty values and encodes the rest', () => {
    expect(buildQuery({ a: 'x y', b: undefined, c: '', d: 0 })).toBe('?a=x+y&d=0');
    expect(buildQuery()).toBe('');
  });
});

describe('request layer', () => {
  it('sends the CSRF header on mutations only', async () => {
    setCsrfToken('tok123');
    const fn = mockFetch(200, { items: [] });
    await api.posts.list();
    await api.posts.publish('p1').catch(() => undefined);
    const calls = fn.mock.calls as unknown as [string, RequestInit][];
    expect((calls[0]![1].headers as Record<string, string>)['X-CSRF-Token']).toBeUndefined();
    expect((calls[1]![1].headers as Record<string, string>)['X-CSRF-Token']).toBe('tok123');
    expect(calls[1]![0]).toBe('/api/v1/posts/p1/publish');
  });

  it('throws ApiError with the server code on failure', async () => {
    mockFetch(409, { error: { code: 'INVALID_STATE_TRANSITION', message: 'nope', request_id: 'r1' } });
    await expect(api.posts.cancel('x')).rejects.toMatchObject({ code: 'INVALID_STATE_TRANSITION', status: 409, requestId: 'r1' });
  });

  it('maps network failure to a NETWORK ApiError', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => { throw new TypeError('fail'); }));
    await expect(api.auth.me()).rejects.toMatchObject({ code: 'NETWORK', status: 0 });
  });

  it('handles 204 with empty body', async () => {
    mockFetch(204, undefined);
    await expect(api.media.remove('m1')).resolves.toBeUndefined();
  });

  it('extracts the one-time raw key and builds MCP config', async () => {
    mockFetch(201, { connection: { id: 'c1', name: 'Claude', scopes: [] }, raw_key: 'sk_live_abc' });
    const c = await api.developer.createMcpConnection({ name: 'Claude', scopes: ['posts:read'] });
    expect(c.rawKey).toBe('sk_live_abc');
    expect(JSON.parse(c.config.http).mcpServers.socialos.headers.Authorization).toBe('Bearer sk_live_abc');
    expect(JSON.parse(c.config.stdio).mcpServers.socialos.env.SOCIALOS_API_KEY).toBe('sk_live_abc');
  });
});
