import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { DemoEngine } from '@/lib/demo/engine';
import { SEEDED_POST_IDS } from '@/lib/demo/ids';
import type { DemoState } from '@/lib/demo/model';
import { buildSeed, DEMO_USER_EMAIL, DEMO_USER_PASSWORD, SEEDED_POST_COUNT } from '@/lib/demo/seed';
import { STORAGE_KEY, loadState, saveState, type StorageLike } from '@/lib/demo/store';
import type { Method } from '@/lib/demo/model';
import type { Post, PostStatus } from '@/lib/types';

const NOW = new Date('2026-10-08T12:00:00Z');
const MIN = 60_000;
const HOUR = 3_600_000;
const DAY = 86_400_000;

interface Rig {
  engine: DemoEngine;
  call: (method: Method, path: string, body?: unknown, query?: Record<string, string | number>) => { status: number; body: any }; // eslint-disable-line @typescript-eslint/no-explicit-any
  tick: (ms: number) => void;
  clock: () => number;
  persisted: DemoState[];
}

function rig(opts: { now?: Date; state?: DemoState; telegramDelayMs?: number } = {}): Rig {
  let clock = (opts.now ?? NOW).getTime();
  const persisted: DemoState[] = [];
  const engine = new DemoEngine(opts.state ?? buildSeed(new Date(clock)), {
    now: () => clock,
    telegramDelayMs: opts.telegramDelayMs,
    onChange: (s) => persisted.push(structuredClone(s)),
  });
  return {
    engine,
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    call: (method, path, body, query) => engine.handle({ method, path, body, query }) as { status: number; body: any },
    tick: (ms) => {
      clock += ms;
    },
    clock: () => clock,
    persisted,
  };
}

const accountId = (r: Rig, provider: string): string => {
  const a = r.engine.state.accounts.find((x) => x.provider === provider);
  if (!a) throw new Error(`no ${provider} account`);
  return a.id;
};

function draft(r: Rig, over: Record<string, unknown> = {}): Post {
  const res = r.call('POST', '/posts', { content: 'Hello from the demo', social_account_ids: [accountId(r, 'linkedin')], ...over });
  expect(res.status).toBe(201);
  return res.body as Post;
}

describe('seed data', () => {
  it('has the demo user, three accounts and stable post ids', () => {
    const s = buildSeed(NOW);
    expect(s.user.email).toBe(DEMO_USER_EMAIL);
    expect(s.user.password).toBe(DEMO_USER_PASSWORD);
    expect(s.signed_in).toBe(true);
    expect(s.accounts.map((a) => a.provider).sort()).toEqual(['linkedin', 'mock', 'telegram']);
    expect(s.posts).toHaveLength(SEEDED_POST_COUNT);
    expect(s.posts.map((p) => p.id)).toEqual([...SEEDED_POST_IDS]);
    expect(new Set(s.posts.map((p) => p.id)).size).toBe(SEEDED_POST_COUNT);
  });

  it('covers every interesting post status', () => {
    const statuses = new Set(buildSeed(NOW).posts.map((p) => p.status));
    for (const st of ['draft', 'scheduled', 'published', 'partially_published', 'failed', 'cancelled'] as PostStatus[]) {
      expect(statuses.has(st)).toBe(true);
    }
  });

  it.each(['2026-10-08T12:00:00Z', '2026-01-31T09:00:00Z', '2026-12-31T23:30:00Z', '2026-03-01T00:10:00Z'])(
    'puts scheduled posts in the future and published ones in the past (%s), across this and next month',
    (iso) => {
      const now = new Date(iso);
      const s = buildSeed(now);
      for (const p of s.posts) {
        if (p.status === 'scheduled') expect(Date.parse(p.scheduled_at ?? '')).toBeGreaterThan(now.getTime());
        if (p.status === 'published') expect(Date.parse(p.published_at ?? '')).toBeLessThanOrEqual(now.getTime());
      }
      const months = new Set(s.posts.filter((p) => p.scheduled_at).map((p) => new Date(p.scheduled_at ?? '').getMonth()));
      expect(months.size).toBeGreaterThanOrEqual(2);
      const thisMonth = s.posts.filter(
        (p) => p.status === 'published' && new Date(p.published_at ?? '').getMonth() === now.getMonth(),
      );
      expect(thisMonth.length).toBeGreaterThan(0);
    },
  );

  it('includes audit logs from users, agents and the scheduler, plus keys and MCP connections', () => {
    const s = buildSeed(NOW);
    expect(new Set(s.audit.map((a) => a.actor_type))).toEqual(new Set(['user', 'api_key', 'scheduler', 'system']));
    expect(s.audit.length).toBeGreaterThan(25);
    expect([...s.audit].sort((a, b) => b.created_at.localeCompare(a.created_at)).map((a) => a.id)).toEqual(s.audit.map((a) => a.id));
    expect(s.api_keys.some((k) => k.revoked_at)).toBe(true);
    expect(s.api_keys.some((k) => !k.revoked_at)).toBe(true);
    expect(s.mcp_connections.length).toBeGreaterThanOrEqual(2);
    expect(s.posts.some((p) => p.created_by === 'api_key')).toBe(true);
  });
});

describe('auth (single built-in user)', () => {
  it('is signed in from the start', () => {
    const r = rig();
    const me = r.call('GET', '/me');
    expect(me.status).toBe(200);
    expect(me.body).toMatchObject({ email: DEMO_USER_EMAIL, display_name: 'Demo User' });
    expect(me.body.scopes).toContain('posts:publish');
  });

  it('rejects wrong credentials and accepts the demo ones after signing out', () => {
    const r = rig();
    expect(r.call('POST', '/auth/logout').status).toBe(204);
    expect(r.call('GET', '/me').status).toBe(401);
    expect(r.call('GET', '/posts').body.error.code).toBe('UNAUTHENTICATED');
    const bad = r.call('POST', '/auth/login', { email: DEMO_USER_EMAIL, password: 'nope' });
    expect(bad.status).toBe(401);
    expect(bad.body.error.code).toBe('UNAUTHENTICATED');
    expect(r.call('POST', '/auth/login', { email: DEMO_USER_EMAIL, password: DEMO_USER_PASSWORD }).status).toBe(200);
    expect(r.call('GET', '/me').status).toBe(200);
  });

  it('validates registration and renames the single user', () => {
    const r = rig();
    expect(r.call('POST', '/auth/register', { email: 'a@b.co', password: 'short' }).status).toBe(400);
    expect(r.call('POST', '/auth/register', { email: DEMO_USER_EMAIL, password: 'longenough1' }).status).toBe(409);
    const ok = r.call('POST', '/auth/register', { email: 'sam@example.com', password: 'longenough1', display_name: 'Sam' });
    expect(ok.status).toBe(201);
    expect(r.call('GET', '/me').body).toMatchObject({ email: 'sam@example.com', display_name: 'Sam' });
    // The seeded data is still there: there are no tenants in the demo.
    expect(r.call('GET', '/social/accounts').body.items).toHaveLength(3);
  });
});

describe('post lifecycle', () => {
  it('draft -> scheduled -> published by the simulated scheduler', () => {
    const r = rig();
    const d = draft(r, { social_account_ids: [accountId(r, 'linkedin'), accountId(r, 'telegram')] });
    expect(d.status).toBe('draft');
    expect(d.targets.map((t) => t.status)).toEqual(['pending', 'pending']);

    const at = new Date(r.clock() + 10 * MIN).toISOString();
    const s = r.call('POST', `/posts/${d.id}/schedule`, { scheduled_at: at });
    expect(s.status).toBe(200);
    expect(s.body.status).toBe('scheduled');
    expect(s.body.scheduled_at).toBe(at);

    r.tick(5 * MIN);
    expect(r.call('GET', `/posts/${d.id}`).body.status).toBe('scheduled');
    r.tick(6 * MIN);
    const done = r.call('GET', `/posts/${d.id}`).body as Post;
    expect(done.status).toBe('published');
    expect(done.published_at).not.toBeNull();
    expect(done.targets.every((t) => t.status === 'published' && !!t.external_url && t.attempt_count === 1)).toBe(true);
    expect(done.attempts).toHaveLength(2);
    expect(done.attempts?.every((a) => a.status === 'succeeded')).toBe(true);
    // The scheduler is named in the audit log.
    const logs = r.call('GET', '/audit-logs', undefined, { limit: 10 }).body.items as { actor_type: string; action: string }[];
    expect(logs.some((l) => l.actor_type === 'scheduler' && l.action === 'post_target.published')).toBe(true);
    expect(r.call('GET', '/dashboard/summary').body.recent.some((p: Post) => p.id === d.id)).toBe(true);
  });

  it('publishes a post whose time passed while nobody was looking (state restored later)', () => {
    const r1 = rig();
    const d = draft(r1);
    r1.call('POST', `/posts/${d.id}/schedule`, { scheduled_at: new Date(r1.clock() + HOUR).toISOString() });
    // The tab is closed; the state sits in localStorage; the visitor returns two hours later.
    const saved = JSON.parse(JSON.stringify(r1.engine.state)) as DemoState;
    const r2 = rig({ state: saved, now: new Date(r1.clock() + 2 * HOUR) });
    const post = r2.call('GET', `/posts/${d.id}`).body as Post;
    expect(post.status).toBe('published');
    // Published at the scheduled time, not at the moment of the visit.
    expect(Date.parse(post.published_at ?? '')).toBeLessThan(r2.clock() - HOUR / 2);
  });

  it('advance() reports whether the scheduler changed anything and persists it', () => {
    const r = rig();
    const d = draft(r);
    r.call('POST', `/posts/${d.id}/schedule`, { scheduled_at: new Date(r.clock() + 2 * MIN).toISOString() });
    const before = r.persisted.length;
    expect(r.engine.advance()).toBe(false);
    r.tick(3 * MIN);
    expect(r.engine.advance()).toBe(true);
    expect(r.persisted.length).toBe(before + 1);
    expect(r.engine.advance()).toBe(false);
  });

  it('publish now settles after the simulated provider call', () => {
    const r = rig();
    const d = draft(r);
    const p = r.call('POST', `/posts/${d.id}/publish`);
    expect(p.body.status).toBe('publishing');
    expect(p.body.targets[0].status).toBe('publishing');
    expect(r.call('GET', `/posts/${d.id}/status`).body.status).toBe('publishing');
    r.tick(1500);
    expect(r.call('GET', `/posts/${d.id}/status`).body.status).toBe('published');
  });

  it('a failing publish ends as failed and can be retried', () => {
    const r = rig();
    const d = draft(r, { content: 'This will FAIL on purpose' });
    r.call('POST', `/posts/${d.id}/publish`);
    r.tick(1500);
    const failed = r.call('GET', `/posts/${d.id}`).body as Post;
    expect(failed.status).toBe('failed');
    expect(failed.targets[0]).toMatchObject({ status: 'failed', error_code: 'PROVIDER_ERROR', attempt_count: 1 });
    expect(r.call('POST', `/posts/${d.id}/retry`).body.status).toBe('publishing');
    r.tick(1500);
    const again = r.call('GET', `/posts/${d.id}`).body as Post;
    expect(again.status).toBe('failed');
    expect(again.targets[0]?.attempt_count).toBe(2);
    expect(again.attempts).toHaveLength(2);
  });

  it('retrying the seeded partially published post only retries the failed target', () => {
    const r = rig();
    const partial = r.engine.state.posts.find((p) => p.status === 'partially_published');
    expect(partial).toBeDefined();
    const id = partial?.id ?? '';
    const before = partial?.targets.find((t) => t.status === 'published')?.attempt_count;
    expect(r.call('POST', `/posts/${id}/retry`).status).toBe(200);
    r.tick(1500);
    const after = r.call('GET', `/posts/${id}`).body as Post;
    expect(after.status).toBe('published');
    expect(after.targets.find((t) => t.platform === 'telegram')?.attempt_count).toBe(before);
  });

  it('enforces the state machine', () => {
    const r = rig();
    const published = r.engine.state.posts.find((p) => p.status === 'published')?.id ?? '';
    const e1 = r.call('POST', `/posts/${published}/cancel`);
    expect(e1.status).toBe(409);
    expect(e1.body.error.code).toBe('INVALID_STATE_TRANSITION');
    expect(r.call('POST', `/posts/${published}/retry`).status).toBe(409);
    const d = draft(r);
    expect(r.call('POST', `/posts/${d.id}/retry`).status).toBe(409);
    expect(r.call('POST', `/posts/${d.id}/schedule`, { scheduled_at: new Date(r.clock() - MIN).toISOString() }).body.error.code).toBe('VALIDATION_ERROR');
    expect(r.call('POST', `/posts/${d.id}/schedule`, {}).status).toBe(400);
    const cancelled = r.call('POST', `/posts/${d.id}/cancel`);
    expect(cancelled.body.status).toBe('cancelled');
    expect(cancelled.body.targets[0].status).toBe('cancelled');
    expect(r.call('POST', `/posts/${d.id}/publish`).status).toBe(409);
  });

  it('deletes posts but not ones that are publishing', () => {
    const r = rig();
    const d = draft(r);
    r.call('POST', `/posts/${d.id}/publish`);
    expect(r.call('DELETE', `/posts/${d.id}`).status).toBe(409);
    r.tick(1500);
    expect(r.call('DELETE', `/posts/${d.id}`).status).toBe(204);
    expect(r.call('GET', `/posts/${d.id}`).status).toBe(404);
  });

  it('creates a scheduled post directly and refuses a time in the past', () => {
    const r = rig();
    const li = accountId(r, 'linkedin');
    const past = r.call('POST', '/posts', { content: 'x', social_account_ids: [li], schedule: true, scheduled_at: new Date(r.clock() - HOUR).toISOString() });
    expect(past.status).toBe(400);
    const ok = r.call('POST', '/posts', { content: 'x', social_account_ids: [li], schedule: true, scheduled_at: new Date(r.clock() + DAY).toISOString() });
    expect(ok.status).toBe(201);
    expect(ok.body.status).toBe('scheduled');
  });

  it('supports per-platform content and validates input like the API does', () => {
    const r = rig();
    const li = accountId(r, 'linkedin');
    const tg = accountId(r, 'telegram');
    const mock = accountId(r, 'mock');
    const tailored = r.call('POST', '/posts', {
      title: 'Tailored',
      content: 'Long version',
      social_account_ids: [li, tg],
      targets: [{ social_account_id: tg, content: 'Short' }],
    });
    expect(tailored.body.targets.map((t: { content: string }) => t.content)).toEqual(['Long version', 'Short']);
    expect(r.call('POST', '/posts', { content: '  ', social_account_ids: [li] }).body.error.code).toBe('VALIDATION_ERROR');
    expect(r.call('POST', '/posts', { content: 'x', social_account_ids: [] }).status).toBe(400);
    expect(r.call('POST', '/posts', { content: 'x', social_account_ids: ['nope'] }).status).toBe(400);
    expect(r.call('POST', '/posts', { content: 'x'.repeat(281), social_account_ids: [mock] }).status).toBe(400);
    expect(r.call('POST', '/posts', { content: 'x'.repeat(281), social_account_ids: [li] }).status).toBe(201);
  });

  it('lists with filters, pagination and a time window', () => {
    const r = rig();
    const drafts = r.call('GET', '/posts', undefined, { status: 'draft' }).body.items as Post[];
    expect(drafts.length).toBe(3);
    expect(drafts.every((p) => p.status === 'draft')).toBe(true);
    const p1 = r.call('GET', '/posts', undefined, { limit: 10 }).body;
    expect(p1.items).toHaveLength(10);
    expect(p1.next_cursor).toBe('10');
    const p3 = r.call('GET', '/posts', undefined, { limit: 10, cursor: '20' }).body;
    expect(p3.items).toHaveLength(SEEDED_POST_COUNT - 20);
    expect(p3.next_cursor).toBeNull();
    const inWindow = r.call('GET', '/posts', undefined, { from: new Date(r.clock()).toISOString(), to: new Date(r.clock() + 3 * DAY).toISOString() }).body.items as Post[];
    expect(inWindow.length).toBeGreaterThan(0);
    for (const p of inWindow) expect(Date.parse(p.scheduled_at ?? '')).toBeGreaterThanOrEqual(r.clock());
    // The list view omits attempts and media; the detail view has them.
    expect(p1.items[0].attempts).toBeUndefined();
    expect(Array.isArray(r.call('GET', `/posts/${SEEDED_POST_IDS[0]}`).body.attempts)).toBe(true);
  });

  it('dashboard summary follows the data', () => {
    const r = rig();
    const before = r.call('GET', '/dashboard/summary').body;
    expect(before).toMatchObject({ connected_accounts: 3, scheduled_posts: 8, drafts: 3, failed: 2 });
    expect(before.upcoming).toHaveLength(5);
    expect(before.upcoming[0].scheduled_at <= before.upcoming[1].scheduled_at).toBe(true);
    draft(r);
    expect(r.call('GET', '/dashboard/summary').body.drafts).toBe(4);
  });

  it('disconnecting an account cancels what was waiting to go out to it', () => {
    const r = rig();
    const li = accountId(r, 'linkedin');
    const d = draft(r);
    r.call('POST', `/posts/${d.id}/schedule`, { scheduled_at: new Date(r.clock() + HOUR).toISOString() });
    expect(r.call('DELETE', `/social/accounts/${li}`).status).toBe(204);
    expect(r.call('GET', `/posts/${d.id}`).body.status).toBe('cancelled');
    expect(r.call('GET', `/social/accounts/${li}`).status).toBe(404);
    expect(r.call('DELETE', `/social/accounts/${li}`).status).toBe(404);
  });
});

describe('connecting accounts in the demo', () => {
  it('instantly connects LinkedIn and Mock, refuses unavailable networks', () => {
    const r = rig();
    const li = r.call('POST', '/social/linkedin/connect');
    expect(li.status).toBe(201);
    expect(li.body).toMatchObject({ provider: 'linkedin', status: 'active' });
    expect(r.call('GET', '/social/accounts').body.items).toHaveLength(4);
    expect(r.call('POST', '/social/mock/connect').status).toBe(201);
    const x = r.call('POST', '/social/x/connect');
    expect(x.status).toBe(501);
    expect(x.body.error.code).toBe('PROVIDER_NOT_AVAILABLE');
    expect(r.call('POST', '/social/telegram/connect').status).toBe(201); // this is the code flow, not instant connect
  });

  it('lists providers with honest capabilities', () => {
    const items = rig().call('GET', '/social/providers').body.items as { provider: string; status: string }[];
    expect(items.find((p) => p.provider === 'linkedin')?.status).toBe('supported');
    expect(items.find((p) => p.provider === 'telegram')?.status).toBe('supported');
    expect(items.filter((p) => p.status === 'unsupported').map((p) => p.provider).sort()).toEqual(
      ['facebook', 'instagram', 'pinterest', 'threads', 'tiktok', 'x', 'youtube'],
    );
  });

  it('Telegram: mints a code, then "sees" it in a demo channel after a few seconds', () => {
    const r = rig({ telegramDelayMs: 4000 });
    const link = r.call('POST', '/social/telegram/connect');
    expect(link.status).toBe(201);
    expect(link.body.code).toMatch(/^SOS-[A-HJKMNP-Z2-9]{8}$/);
    expect(link.body.bot_username).toBe('socialos_bot');
    const id = link.body.id as string;
    expect(r.call('GET', `/social/telegram/connect/${id}`).body.status).toBe('pending');
    r.tick(4100);
    const done = r.call('GET', `/social/telegram/connect/${id}`).body;
    expect(done.status).toBe('connected');
    expect(done.account).toMatchObject({ provider: 'telegram', status: 'active' });
    expect(r.call('GET', '/social/accounts').body.items).toHaveLength(4);
    // Polling again does not add a second account.
    expect(r.call('GET', `/social/telegram/connect/${id}`).body.status).toBe('connected');
    expect(r.call('GET', '/social/accounts').body.items).toHaveLength(4);
  });

  it('Telegram: codes expire after 15 minutes, only 3 stay active, unknown ids are 404', () => {
    const r = rig({ telegramDelayMs: 0 });
    const first = r.call('POST', '/social/telegram/connect').body.id as string;
    r.call('POST', '/social/telegram/connect');
    r.call('POST', '/social/telegram/connect');
    r.call('POST', '/social/telegram/connect');
    expect(r.call('GET', `/social/telegram/connect/${first}`).body.status).toBe('expired');
    const last = r.call('POST', '/social/telegram/connect').body.id as string;
    expect(r.call('GET', `/social/telegram/connect/${last}`).body.status).toBe('pending');
    r.tick(16 * MIN);
    expect(r.call('GET', `/social/telegram/connect/${last}`).body.status).toBe('expired');
    expect(r.call('GET', '/social/telegram/connect/unknown').status).toBe(404);
  });
});

describe('media, analytics, audit and developer endpoints', () => {
  it('uploads and deletes media with the real type rules', () => {
    const r = rig();
    const bad = r.engine.handle({ method: 'POST', path: '/media', upload: { name: 'x.exe', mime: 'application/x-msdownload', size: 10 } });
    expect(bad.status).toBe(400);
    expect(r.engine.handle({ method: 'POST', path: '/media' }).status).toBe(400);
    expect(r.engine.handle({ method: 'POST', path: '/media', upload: { name: 'big.png', mime: 'image/png', size: 11 * 1024 * 1024 } }).status).toBe(400);
    const ok = r.engine.handle({ method: 'POST', path: '/media', upload: { name: 'pic.png', mime: 'image/png', size: 1000 } });
    expect(ok.status).toBe(201);
    const m = ok.body as { id: string; kind: string; url?: string };
    expect(m.kind).toBe('image');
    expect(m.url).toMatch(/^data:image\//);
    expect(r.call('GET', '/media').body.items[0].id).toBe(m.id);
    expect(r.call('DELETE', `/media/${m.id}`).status).toBe(204);
    expect(r.call('GET', `/media/${m.id}`).status).toBe(404);
  });

  it('serves analytics for the Mock network and respects the window', () => {
    const r = rig();
    const all = r.call('GET', '/analytics').body.items as { metric: string; captured_at: string }[];
    expect(new Set(all.map((p) => p.metric))).toEqual(new Set(['impressions', 'reactions', 'link_clicks']));
    const from = new Date(r.clock() - 7 * DAY).toISOString();
    const week = r.call('GET', '/analytics', undefined, { from, to: new Date(r.clock()).toISOString() }).body.items as { captured_at: string }[];
    expect(week.length).toBeGreaterThan(0);
    expect(week.length).toBeLessThan(all.length);
    expect(week.every((p) => p.captured_at >= from)).toBe(true);
    // Same inputs, same numbers.
    expect(r.call('GET', '/analytics').body.items).toEqual(all);
    r.call('DELETE', `/social/accounts/${accountId(r, 'mock')}`);
    expect(r.call('GET', '/analytics').body.items).toEqual([]);
  });

  it('records every action in the audit log, newest first, with pagination', () => {
    const r = rig();
    const d = draft(r);
    r.call('POST', `/posts/${d.id}/cancel`);
    const page = r.call('GET', '/audit-logs', undefined, { limit: 3 }).body;
    expect(page.items.slice(0, 2).map((l: { action: string }) => l.action)).toEqual(['post.cancelled', 'post.created']);
    expect(page.items[0].actor_label).toBe('Demo User');
    expect(page.next_cursor).toBe('3');
    expect(r.call('GET', '/audit-logs', undefined, { limit: 3, cursor: '3' }).body.items).toHaveLength(3);
  });

  it('creates API keys and MCP connections; the raw key is returned once and never listed', () => {
    const r = rig();
    const k = r.call('POST', '/developer/api-keys', { name: 'Zapier', scopes: ['posts:read', 'social:read'] });
    expect(k.status).toBe(201);
    expect(k.body.raw_key).toMatch(/^sk_live_/);
    expect(k.body.key.prefix).toBe(k.body.raw_key.slice(0, 12));
    const listed = r.call('GET', '/developer/api-keys').body.items as { id: string; name: string; revoked_at: string | null }[];
    expect(JSON.stringify(listed)).not.toContain(k.body.raw_key);
    expect(r.call('POST', '/developer/api-keys', { name: '', scopes: ['posts:read'] }).status).toBe(400);
    expect(r.call('POST', '/developer/api-keys', { name: 'x', scopes: ['root'] }).status).toBe(400);
    expect(r.call('DELETE', `/developer/api-keys/${k.body.key.id}`).status).toBe(204);
    expect(r.call('GET', '/developer/api-keys').body.items.find((x: { id: string }) => x.id === k.body.key.id).revoked_at).not.toBeNull();

    const c = r.call('POST', '/developer/mcp-connections', { name: 'My agent', scopes: ['social:read', 'posts:read', 'posts:write'] });
    expect(c.status).toBe(201);
    expect(c.body.connection).toMatchObject({ name: 'My agent', revoked_at: null });
    expect(c.body.raw_key).toMatch(/^sk_live_.*demo/);
    expect(r.call('POST', '/developer/mcp-connections', { name: 'x', scopes: [] }).status).toBe(400);
    expect(r.call('GET', '/developer/mcp-connections').body.items).toHaveLength(4);
    expect(r.call('DELETE', `/developer/mcp-connections/${c.body.connection.id}`).status).toBe(204);
    expect(r.call('DELETE', '/developer/mcp-connections/missing').status).toBe(404);
    const logs = r.call('GET', '/audit-logs', undefined, { limit: 10 }).body.items as { action: string }[];
    expect(logs.map((l) => l.action)).toEqual(expect.arrayContaining(['api_key.created', 'api_key.revoked', 'mcp_connection.created', 'mcp_connection.revoked']));
  });

  it('usage adds up', () => {
    const u = rig().call('GET', '/developer/usage').body;
    expect(u.total_requests).toBeGreaterThan(0);
    expect(u.by_day.reduce((s: number, d: { requests: number }) => s + d.requests, 0)).toBe(u.total_requests);
    expect(u.by_key.reduce((s: number, k: { requests: number }) => s + k.requests, 0)).toBe(u.total_requests);
  });

  it('unknown routes are 404 in the uniform error format', () => {
    const e = rig().call('GET', '/nope');
    expect(e.status).toBe(404);
    expect(e.body.error).toMatchObject({ code: 'NOT_FOUND' });
    expect(typeof e.body.error.request_id).toBe('string');
  });
});

describe('persistence', () => {
  function fakeStorage(initial?: string): StorageLike & { data: Map<string, string> } {
    const data = new Map<string, string>(initial === undefined ? [] : [[STORAGE_KEY, initial]]);
    return {
      data,
      getItem: (k) => data.get(k) ?? null,
      setItem: (k, v) => void data.set(k, v),
      removeItem: (k) => void data.delete(k),
    };
  }

  it('round-trips the state', () => {
    const st = fakeStorage();
    const s = buildSeed(NOW);
    s.posts[0] && (s.posts[0].title = 'Edited');
    saveState(st, s);
    const back = loadState(st, new Date(NOW.getTime() + HOUR));
    expect(back.posts[0]?.title).toBe('Edited');
    expect(back.seeded_at).toBe(s.seeded_at);
  });

  it('starts fresh when nothing, garbage or a stale copy is stored', () => {
    const fresh = loadState(fakeStorage(), NOW);
    expect(fresh.posts).toHaveLength(SEEDED_POST_COUNT);
    expect(loadState(fakeStorage('{not json'), NOW).posts).toHaveLength(SEEDED_POST_COUNT);
    expect(loadState(fakeStorage('{"version":1}'), NOW).posts).toHaveLength(SEEDED_POST_COUNT);
    const old = fakeStorage();
    saveState(old, buildSeed(new Date(NOW.getTime() - 30 * DAY)));
    expect(loadState(old, NOW).seeded_at).toBe(NOW.toISOString());
  });

  it('never throws when storage is blocked or full', () => {
    const blocked: StorageLike = {
      getItem: () => {
        throw new Error('blocked');
      },
      setItem: () => {
        throw new Error('quota');
      },
      removeItem: () => {
        throw new Error('blocked');
      },
    };
    expect(() => saveState(blocked, buildSeed(NOW))).not.toThrow();
    expect(loadState(blocked, NOW).posts).toHaveLength(SEEDED_POST_COUNT);
    expect(loadState(null, NOW).posts).toHaveLength(SEEDED_POST_COUNT);
  });

  it('the engine hands every change to onChange', () => {
    const r = rig();
    r.call('GET', '/posts');
    expect(r.persisted).toHaveLength(0);
    draft(r);
    expect(r.persisted.length).toBeGreaterThan(0);
    expect(r.persisted.at(-1)?.posts.length).toBe(SEEDED_POST_COUNT + 1);
  });
});

describe('API client in demo mode', () => {
  beforeEach(() => {
    vi.stubEnv('NEXT_PUBLIC_DEMO', 'true');
    window.localStorage.clear();
  });
  afterEach(async () => {
    const { resetDemo } = await import('@/lib/demo');
    resetDemo();
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
  });

  it('answers from the in-browser mock without touching the network', async () => {
    const fetchSpy = vi.fn(async () => new Response('{}'));
    vi.stubGlobal('fetch', fetchSpy);
    const { api, ApiError } = await import('@/lib/api');

    const me = await api.auth.me();
    expect(me.email).toBe(DEMO_USER_EMAIL);
    const providers = await api.social.providers();
    expect(providers.find((p) => p.id === 'linkedin')?.available).toBe(true);
    expect(providers.find((p) => p.id === 'x')?.available).toBe(false);
    const accounts = await api.social.accounts();
    expect(accounts).toHaveLength(3);

    const created = await api.posts.create({ content: 'Through the client', social_account_ids: [accounts[0]?.id ?? ''] });
    expect(created.status).toBe('draft');
    await api.posts.schedule(created.id, new Date(Date.now() + HOUR).toISOString());
    expect((await api.posts.get(created.id)).status).toBe('scheduled');
    await expect(api.posts.cancel('does-not-exist')).rejects.toMatchObject({ code: 'NOT_FOUND', status: 404 });
    await expect(api.posts.schedule(created.id, new Date(Date.now() + HOUR).toISOString())).rejects.toBeInstanceOf(ApiError);

    const mcp = await api.developer.createMcpConnection({ name: 'Test agent', scopes: ['posts:read'] });
    expect(mcp.rawKey).toMatch(/^sk_live_/);
    expect(JSON.parse(mcp.config.http).mcpServers.socialos.headers.Authorization).toBe(`Bearer ${mcp.rawKey}`);

    const summary = await api.dashboard.summary();
    expect(summary.connected_accounts).toBe(3);
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it('persists to localStorage and survives a reset', async () => {
    const { api } = await import('@/lib/api');
    const { resetDemo } = await import('@/lib/demo');
    const acc = (await api.social.accounts())[0];
    await api.posts.create({ content: 'Saved', social_account_ids: [acc?.id ?? ''] });
    const stored = JSON.parse(window.localStorage.getItem(STORAGE_KEY) ?? '{}') as DemoState;
    expect(stored.posts.some((p) => p.targets[0]?.content === 'Saved')).toBe(true);
    resetDemo();
    expect(window.localStorage.getItem(STORAGE_KEY)).toBeNull();
    const after = await api.posts.list({ limit: 100 });
    expect(after.items.some((p) => p.content === 'Saved')).toBe(false);
    expect(after.items).toHaveLength(SEEDED_POST_COUNT);
  });

  it('sign out and sign in work through the client', async () => {
    const { api } = await import('@/lib/api');
    await api.auth.logout();
    await expect(api.auth.me()).rejects.toMatchObject({ status: 401 });
    await expect(api.auth.login({ email: DEMO_USER_EMAIL, password: 'wrong' })).rejects.toMatchObject({ code: 'UNAUTHENTICATED' });
    expect((await api.auth.login({ email: DEMO_USER_EMAIL, password: DEMO_USER_PASSWORD })).email).toBe(DEMO_USER_EMAIL);
  });
});
