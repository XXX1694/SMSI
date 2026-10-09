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
    expect(JSON.parse(c.config.stdio).mcpServers.socialos.env.SOCIALOS_AUTH_HEADER).toBe("Bearer sk_live_abc");
  });
});

describe('telegram link flow', () => {
  it('starts a link with a bodyless CSRF-protected POST (no chat is ever sent)', async () => {
    setCsrfToken('tok123');
    const fn = mockFetch(201, { id: 'l1', code: 'SOS-7KQ2M9XA', expires_at: '2026-10-07T12:15:00Z', bot_username: 'socialos_bot', instructions: 'x' });
    const link = await api.social.startTelegramLink();
    const [url, init] = (fn.mock.calls as unknown as [string, RequestInit][])[0]!;
    expect(url).toBe('/api/v1/social/telegram/connect');
    expect(init.method).toBe('POST');
    expect(init.body).toBeUndefined();
    expect((init.headers as Record<string, string>)['X-CSRF-Token']).toBe('tok123');
    expect(link).toMatchObject({ id: 'l1', code: 'SOS-7KQ2M9XA', bot_username: 'socialos_bot' });
  });

  it('polls the link status by id and encodes the id', async () => {
    const fn = mockFetch(200, { status: 'connected', account: { id: 'a1', provider: 'telegram', username: 'chan' } });
    const st = await api.social.telegramLinkStatus('a/b');
    expect((fn.mock.calls as unknown as [string][])[0]![0]).toBe('/api/v1/social/telegram/connect/a%2Fb');
    expect(st.status).toBe('connected');
    expect(st.account?.id).toBe('a1');
  });

  it('surfaces a 404 for somebody else’s link as an ApiError', async () => {
    mockFetch(404, { error: { code: 'NOT_FOUND', message: 'not found' } });
    await expect(api.social.telegramLinkStatus('other')).rejects.toMatchObject({ status: 404 });
  });
});

describe('auth recovery endpoints', () => {
  const lastCall = (fn: ReturnType<typeof mockFetch>) => {
    const call = fn.mock.calls[0] as unknown as [string, RequestInit];
    return { url: call[0], init: call[1], body: JSON.parse(String(call[1].body ?? 'null')) as unknown };
  };

  it('verifies, resends, requests and completes a reset, and changes the password', async () => {
    setCsrfToken('csrf-1');
    let fn = mockFetch(200, { email_verified: true });
    await api.auth.verifyEmail('tok');
    expect(lastCall(fn)).toMatchObject({ url: '/api/v1/auth/verify-email', body: { token: 'tok' } });
    expect(lastCall(fn).init.method).toBe('POST');

    fn = mockFetch(202, { status: 'accepted', delivery: 'smtp' });
    await api.auth.resendVerification();
    expect(lastCall(fn).url).toBe('/api/v1/auth/verify-email/resend');

    fn = mockFetch(202, { status: 'accepted', delivery: 'log' });
    await expect(api.auth.forgotPassword('a@example.com')).resolves.toEqual({ delivery: 'log' });
    expect(lastCall(fn)).toMatchObject({ url: '/api/v1/auth/password/forgot', body: { email: 'a@example.com' } });

    fn = mockFetch(204, undefined);
    await api.auth.resetPassword('tok', 'new password');
    expect(lastCall(fn)).toMatchObject({ url: '/api/v1/auth/password/reset', body: { token: 'tok', password: 'new password', revoke_keys: false } });

    fn = mockFetch(204, undefined);
    await api.auth.changePassword('old', 'new password');
    expect(lastCall(fn)).toMatchObject({ url: '/api/v1/auth/password/change', body: { current_password: 'old', new_password: 'new password', revoke_keys: false } });
    expect((lastCall(fn).init.headers as Record<string, string>)['X-CSRF-Token']).toBe('csrf-1');

    fn = mockFetch(204, undefined);
    await api.auth.changePassword('old', 'new password', true);
    expect(lastCall(fn).body).toMatchObject({ revoke_keys: true });
  });

  it('turns the server /me shape into Me, defaulting to "nothing restricted" for older servers', async () => {
    mockFetch(200, {
      id: 'u', email: 'a@example.com', display_name: 'A', csrf_token: 'c', scopes: ['posts:read'],
      user: { email_verified: false, plan: 'free' }, verification_enforced: true, mail_delivery: 'smtp',
    });
    await expect(api.auth.me()).resolves.toMatchObject({ email_verified: false, verification_enforced: true, mail_delivery: 'smtp' });
    mockFetch(200, { id: 'u', email: 'a@example.com', display_name: 'A', csrf_token: 'c' });
    await expect(api.auth.me()).resolves.toMatchObject({ email_verified: true, verification_enforced: false, mail_delivery: 'smtp' });
    mockFetch(200, { id: 'u', email: 'a@example.com', display_name: 'A', csrf_token: 'c', mail_delivery: 'log' });
    await expect(api.auth.me()).resolves.toMatchObject({ mail_delivery: 'log' });
  });

  it('surfaces the 403 EMAIL_NOT_VERIFIED code', async () => {
    mockFetch(403, { error: { code: 'EMAIL_NOT_VERIFIED', message: 'verify your email address to use this feature', request_id: 'r1' } });
    await expect(api.posts.publish('p1')).rejects.toMatchObject({ status: 403, code: 'EMAIL_NOT_VERIFIED' });
  });
});

describe('approvals endpoints', () => {
  it('lists pending ones by default, asks for history with status=all, and decides with POST + CSRF', async () => {
    setCsrfToken('tok123');
    const fn = mockFetch(200, { items: [{ id: 'a1' }], next_cursor: 'c2' });
    const page = await api.approvals.list();
    expect(page.items).toHaveLength(1);
    expect(page.next_cursor).toBe('c2');
    await api.approvals.list('all', 10, 'c2');
    await api.approvals.approve('a/1');
    await api.approvals.deny('a2');
    const calls = fn.mock.calls as unknown as [string, RequestInit][];
    expect(calls.map((c) => `${c[1].method} ${c[0]}`)).toEqual([
      'GET /api/v1/approvals?status=pending&limit=25',
      'GET /api/v1/approvals?status=all&limit=10&cursor=c2',
      'POST /api/v1/approvals/a%2F1/approve',
      'POST /api/v1/approvals/a2/deny',
    ]);
    expect((calls[2]![1].headers as Record<string, string>)['X-CSRF-Token']).toBe('tok123');
  });

  it('a request that is no longer pending is a 409 ApiError', async () => {
    mockFetch(409, { error: { code: 'CONFLICT', message: 'this approval is no longer pending', request_id: 'r' } });
    await expect(api.approvals.approve('a1')).rejects.toMatchObject({ code: 'CONFLICT', status: 409 });
  });
});

describe('parseErrorBody fields', () => {
  it('keeps per-field messages and ignores non-string values', () => {
    const e = parseErrorBody(400, { error: { code: 'VALIDATION_ERROR', message: 'm', fields: { a: 'required', b: 3 } } });
    expect(e.fields).toEqual({ a: 'required' });
    expect(parseErrorBody(500, null).fields).toEqual({});
  });
});
