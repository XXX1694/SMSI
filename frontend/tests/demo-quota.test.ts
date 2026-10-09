import { describe, expect, it } from 'vitest';
import { DemoEngine } from '@/lib/demo/engine';
import type { Method } from '@/lib/demo/model';
import { DEMO_LIMITS } from '@/lib/demo/quota';
import { buildSeed } from '@/lib/demo/seed';

const NOW = new Date('2026-10-08T12:00:00Z');

function rig() {
  const engine = new DemoEngine(buildSeed(NOW), { now: () => NOW.getTime() });
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const call = (method: Method, path: string, body?: unknown, upload?: { name: string; mime: string; size: number }) => engine.handle({ method, path, body, upload }) as { status: number; body: any };
  return { engine, call };
}

describe('demo plan limits', () => {
  it('reports the free plan with usage counted from the demo data', () => {
    const { call } = rig();
    const u = call('GET', '/account/usage').body;
    const accounts = call('GET', '/social/accounts').body.items.length;
    expect(u).toMatchObject({ plan: 'free', period_start: '2026-10-01T00:00:00.000Z', period_end: '2026-11-01T00:00:00.000Z' });
    expect(u.quotas.connected_accounts).toEqual({ used: accounts, limit: DEMO_LIMITS.accounts });
    expect(u.quotas.agent_requests_per_minute).toEqual({ limit: DEMO_LIMITS.agentRpm });
  });

  it('refuses the account over the limit with QUOTA_EXCEEDED and frees the slot on disconnect', () => {
    const { call } = rig();
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    let last: { status: number; body: any } = { status: 201, body: {} };
    for (let i = 0; i < DEMO_LIMITS.accounts + 1 && last.status === 201; i++) last = call('POST', '/social/mock/connect');
    expect(last.status).toBe(403);
    expect(last.body.error.code).toBe('QUOTA_EXCEEDED');
    expect(last.body.error.message).toMatch(/Disconnect an account/);
    const first = call('GET', '/social/accounts').body.items[0].id;
    expect(call('DELETE', `/social/accounts/${first}`).status).toBe(204);
    expect(call('POST', '/social/mock/connect').status).toBe(201);
  });

  it('refuses an upload that does not fit', () => {
    const { call } = rig();
    const r = call('POST', '/media', undefined, { name: 'big.mp4', mime: 'video/mp4', size: 90 * 1024 * 1024 });
    let status = r.status;
    for (let i = 0; i < 6 && status === 201; i++) status = call('POST', '/media', undefined, { name: 'big.mp4', mime: 'video/mp4', size: 90 * 1024 * 1024 }).status;
    expect(status).toBe(403);
  });
});
