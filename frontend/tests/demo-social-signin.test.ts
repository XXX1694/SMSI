import { describe, expect, it } from 'vitest';
import { DemoEngine } from '@/lib/demo/engine';
import { buildSeed } from '@/lib/demo/seed';

const NOW = new Date('2026-10-08T12:00:00Z');
const fresh = () => {
  const engine = new DemoEngine(buildSeed(NOW), { now: () => NOW.getTime() });
  const call = (method: 'GET' | 'POST', path: string, body?: unknown, query?: Record<string, string>) =>
    engine.handle({ method, path, body, query }) as { status: number; body: any }; // eslint-disable-line @typescript-eslint/no-explicit-any
  return { call };
};

describe('demo provider sign-in', () => {
  it('lists GitHub and Google without a session', () => {
    const { call } = fresh();
    const res = call('GET', '/auth/providers');
    expect(res.status).toBe(200);
    expect(res.body.providers).toEqual([{ id: 'github', name: 'GitHub' }, { id: 'google', name: 'Google' }]);
  });

  it('start answers with the sign-up page for a new person; pending shows what will be created', () => {
    const { call } = fresh();
    expect(call('GET', '/auth/oauth/pending').status).toBe(404);
    expect(call('GET', '/auth/oauth/github/start', undefined, { next: '/posts' }).body).toEqual({ redirect: '/signup/complete' });
    const pending = call('GET', '/auth/oauth/pending');
    expect(pending.status).toBe(200);
    expect(pending.body).toMatchObject({ provider: 'github', email: 'sam.rivera@example.com', display_name: 'Sam Rivera', next: '/posts' });
  });

  it('refuses an unknown provider and an off-site next', () => {
    const { call } = fresh();
    expect(call('GET', '/auth/oauth/myspace/start').status).toBe(404);
    for (const bad of ['//evil.example', '/\t/evil.example', '/\n/evil.example']) {
      call('GET', '/auth/oauth/google/start', undefined, { next: bad });
      expect(call('GET', '/auth/oauth/pending').body.next).toBe('/dashboard');
    }
  });

  it('complete needs the Terms, then signs in and forgets the ticket', () => {
    const { call } = fresh();
    expect(call('POST', '/auth/oauth/complete', { accept_terms: true }).status).toBe(404);
    call('GET', '/auth/oauth/google/start');
    const refused = call('POST', '/auth/oauth/complete', { display_name: 'Sam', accept_terms: false });
    expect(refused.status).toBe(400);
    expect(refused.body.error.fields.accept_terms).toBeDefined();
    const created = call('POST', '/auth/oauth/complete', { display_name: 'Sam', accept_terms: true });
    expect(created.status).toBe(201);
    expect(created.body).toMatchObject({ email: 'sam.rivera@example.com', display_name: 'Sam' });
    expect(call('GET', '/me').status).toBe(200);
    expect(call('GET', '/auth/oauth/pending').status).toBe(404);
  });

  it('rejects a name over 100 characters on the name field', () => {
    const { call } = fresh();
    call('GET', '/auth/oauth/github/start');
    const res = call('POST', '/auth/oauth/complete', { display_name: 'x'.repeat(101), accept_terms: true });
    expect(res.status).toBe(400);
    expect(res.body.error.fields.display_name).toBeDefined();
  });
});
