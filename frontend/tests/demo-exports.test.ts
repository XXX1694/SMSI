import { describe, expect, it } from 'vitest';
import { DemoEngine } from '@/lib/demo/engine';
import type { Method } from '@/lib/demo/model';
import { buildSeed } from '@/lib/demo/seed';

const T0 = new Date('2026-10-08T12:00:00Z').getTime();

function rig() {
  let now = T0;
  const engine = new DemoEngine(buildSeed(new Date(T0)), { now: () => now });
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const call = (method: Method, path: string) => engine.handle({ method, path }) as { status: number; body: any };
  return { call, wait: (ms: number) => (now += ms) };
}

describe('demo data export', () => {
  it('starts empty, prepares for a few seconds, then offers a link', () => {
    const { call, wait } = rig();
    expect(call('GET', '/account/exports').body.items).toEqual([]);
    const created = call('POST', '/account/exports');
    expect(created.status).toBe(202);
    expect(created.body.status).toBe('pending');
    expect(call('GET', `/account/exports/${created.body.id}`).status).toBe(409);
    wait(7000);
    const [ready] = call('GET', '/account/exports').body.items;
    expect(ready).toMatchObject({ id: created.body.id, status: 'ready' });
    expect(ready.size_bytes).toBeGreaterThan(0);
    const link = call('GET', `/account/exports/${created.body.id}`).body;
    expect(link.url).toMatch(/^data:application\/json/);
    // A credential never ends up in the demo file either.
    expect(decodeURIComponent(link.url)).not.toMatch(/password|token|key_hash/i);
  });

  it('keeps the same rules as the API: one at a time, a 24 hour cooldown, expiry after the retention', () => {
    const { call, wait } = rig();
    call('POST', '/account/exports');
    expect(call('POST', '/account/exports').status).toBe(409);
    wait(7000);
    const limited = call('POST', '/account/exports');
    expect(limited.status).toBe(429);
    expect(limited.body.error.code).toBe('RATE_LIMITED');
    wait(25 * 3600_000);
    expect(call('POST', '/account/exports').status).toBe(202);
    wait(8 * 86_400_000);
    expect(call('GET', '/account/exports').body.items.map((e: { status: string }) => e.status)).toContain('expired');
  });

  it('answers an unknown export with 404', () => {
    expect(rig().call('GET', '/account/exports/nope').status).toBe(404);
  });
});
