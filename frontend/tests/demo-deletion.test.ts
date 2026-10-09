import { describe, expect, it } from 'vitest';
import { DemoEngine } from '@/lib/demo/engine';
import type { Method } from '@/lib/demo/model';
import { buildSeed } from '@/lib/demo/seed';

const NOW = new Date('2026-10-08T12:00:00Z');

function rig() {
  const engine = new DemoEngine(buildSeed(NOW), { now: () => NOW.getTime() });
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const call = (method: Method, path: string, body?: unknown) => engine.handle({ method, path, body }) as { status: number; body: any };
  return { call };
}
const ok = { password: 'demo12345', confirm: 'demo@socialos.dev' };

describe('demo account deletion', () => {
  it('needs the right password and the typed email, and names the field', () => {
    const { call } = rig();
    expect(call('POST', '/account/delete', { ...ok, password: 'wrong' }).body.error.fields).toEqual({ password: 'incorrect' });
    expect(call('POST', '/account/delete', { ...ok, confirm: 'other@x.dev' }).body.error.fields.confirm).toBeTruthy();
    expect(call('GET', '/me').body.user.deletion_scheduled_at).toBeNull();
  });

  it('schedules for 7 days, ends the session, and signing in shows it so it can be cancelled', () => {
    const { call } = rig();
    const r = call('POST', '/account/delete', ok);
    expect(r.status).toBe(202);
    expect(Date.parse(r.body.scheduled_for) - NOW.getTime()).toBe(7 * 86_400_000);
    expect(call('GET', '/me').status).toBe(401);
    const login = call('POST', '/auth/login', { email: ok.confirm, password: ok.password });
    expect(login.body.user.deletion_scheduled_at).toBe(r.body.scheduled_for);
    expect(login.body.deletion_grace_days).toBe(7);
    expect(call('POST', '/account/delete', ok).status).toBe(409);
    expect(call('POST', '/account/delete/cancel').status).toBe(204);
    expect(call('GET', '/me').body.user.deletion_scheduled_at).toBeNull();
    expect(call('POST', '/account/delete/cancel').status).toBe(409);
  });

  it('sets scheduled posts back to drafts', () => {
    const { call } = rig();
    const before = call('GET', '/posts', undefined).body.items.filter((p: { status: string }) => p.status === 'scheduled').length;
    expect(before).toBeGreaterThan(0);
    call('POST', '/account/delete', ok);
    call('POST', '/auth/login', { email: ok.confirm, password: ok.password });
    expect(call('GET', '/posts', undefined).body.items.filter((p: { status: string }) => p.status === 'scheduled')).toHaveLength(0);
  });
});
