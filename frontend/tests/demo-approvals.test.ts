import { describe, expect, it } from 'vitest';
import { DemoEngine } from '@/lib/demo/engine';
import type { DemoState, Method } from '@/lib/demo/model';
import { buildSeed } from '@/lib/demo/seed';
import { STORAGE_KEY, loadState, type StorageLike } from '@/lib/demo/store';
import type { Approval } from '@/lib/types';

const NOW = new Date('2026-10-08T12:00:00Z');

function rig(state: DemoState = buildSeed(NOW)) {
  let clock = NOW.getTime();
  const engine = new DemoEngine(state, { now: () => clock });
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const call = (method: Method, path: string, query?: Record<string, string | number>) => engine.handle({ method, path, query }) as { status: number; body: any };
  return { engine, call, advance: (ms: number) => { clock += ms; } };
}

describe('demo approvals endpoints', () => {
  it('lists the waiting requests newest first and the whole history with status=all', () => {
    const r = rig();
    const pending = r.call('GET', '/approvals').body.items as Approval[];
    expect(pending.map((a) => a.status)).toEqual(['pending', 'pending']);
    expect(pending[0]!.created_at > pending[1]!.created_at).toBe(true);
    const all = r.call('GET', '/approvals', { status: 'all' }).body.items as Approval[];
    expect(all).toHaveLength(3);
    expect(all.some((a) => a.status === 'denied')).toBe(true);
  });

  it('approves and denies once, audits it, and answers 409 for a decided request and 404 for an unknown one', () => {
    const r = rig();
    const [first, second] = r.call('GET', '/approvals').body.items as Approval[];
    const ok = r.call('POST', `/approvals/${first!.id}/approve`);
    expect(ok.status).toBe(200);
    expect(ok.body).toMatchObject({ id: first!.id, status: 'approved' });
    expect(ok.body.decided_at).toBe(NOW.toISOString());
    expect(r.call('POST', `/approvals/${first!.id}/deny`).status).toBe(409);
    expect(r.call('POST', `/approvals/${second!.id}/deny`).body.status).toBe('denied');
    expect(r.call('POST', '/approvals/00000000-0000-4000-8000-00000000dead/approve').status).toBe(404);
    expect(r.call('GET', '/approvals').body.items).toEqual([]);
    const actions = (r.call('GET', '/audit-logs', { limit: 5 }).body.items as { action: string }[]).map((a) => a.action);
    expect(actions.slice(0, 2).sort()).toEqual(['approval.approved', 'approval.denied']);
  });

  it('a request that ran out of time is no longer pending and cannot be approved', () => {
    const r = rig();
    const [first] = r.call('GET', '/approvals').body.items as Approval[];
    r.advance(11 * 60_000);
    expect(r.call('GET', '/approvals').body.items).toEqual([]);
    expect((r.call('GET', '/approvals', { status: 'all' }).body.items as Approval[]).find((a) => a.id === first!.id)?.status).toBe('expired');
    expect(r.call('POST', `/approvals/${first!.id}/approve`).status).toBe(409);
  });

  it('needs a session like the rest of the API', () => {
    const r = rig();
    r.call('POST', '/auth/logout');
    expect(r.call('GET', '/approvals').status).toBe(401);
  });

  it('a demo saved before approvals existed gets the sample requests when it is restored', () => {
    const old = structuredClone(buildSeed(new Date())) as Partial<DemoState>;
    delete old.approvals;
    const store = new Map([[STORAGE_KEY, JSON.stringify(old)]]);
    const storage: StorageLike = { getItem: (k) => store.get(k) ?? null, setItem: (k, v) => void store.set(k, v), removeItem: (k) => void store.delete(k) };
    expect(loadState(storage).approvals.length).toBeGreaterThan(0);
  });

  it('a demo saved under the old login email is restored with the new one', () => {
    const old = structuredClone(buildSeed(new Date()));
    old.user.email = 'demo@socialos.dev';
    const store = new Map([[STORAGE_KEY, JSON.stringify(old)]]);
    const storage: StorageLike = { getItem: (k) => store.get(k) ?? null, setItem: (k, v) => void store.set(k, v), removeItem: (k) => void store.delete(k) };
    const restored = loadState(storage);
    expect(restored.user.email).toBe('demo@example.com');
    expect(restored.posts.length).toBe(old.posts.length);
  });
});
