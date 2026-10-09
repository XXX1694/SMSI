import { describe, expect, it } from 'vitest';
import { DemoEngine } from '@/lib/demo/engine';
import { buildSeed } from '@/lib/demo/seed';

const NOW = new Date('2026-10-08T12:00:00Z');
const make = () => {
  const engine = new DemoEngine(buildSeed(NOW), { now: () => NOW.getTime() });
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const call = (method: any, path: string, body?: unknown) => engine.handle({ method, path, body }) as { status: number; body: any }; // eslint-disable-line @typescript-eslint/no-explicit-any
  return { call };
};

describe('demo PATCH /posts/{id}', () => {
  it('edits a draft: text, overrides, accounts and title, keeping target ids and bumping updated_at', () => {
    const { call } = make();
    const accounts = call('GET', '/social/accounts').body as { id: string }[] | { items: { id: string }[] };
    const list = Array.isArray(accounts) ? accounts : accounts.items;
    const [a, b] = list;
    const created = call('POST', '/posts', { title: 'T', content: 'Base', social_account_ids: [a!.id] }).body;
    const patched = call('PATCH', `/posts/${created.id}`, {
      title: 'New', content: 'Base 2', social_account_ids: [a!.id, b!.id], targets: [{ social_account_id: b!.id, content: 'Only B' }],
    });
    expect(patched.status).toBe(200);
    expect(patched.body).toMatchObject({ title: 'New', content: 'Base 2', status: 'draft' });
    expect(patched.body.targets.map((t: { content: string }) => t.content)).toEqual(['Base 2', 'Only B']);
    expect(patched.body.targets[0].id).toBe(created.targets[0].id);
    expect(call('GET', `/posts/${created.id}`).body.content).toBe('Base 2');
  });

  it('moves a scheduled post, refuses a past time, and refuses scheduled_at on a draft', () => {
    const { call } = make();
    const acc = call('GET', '/social/accounts').body;
    const id = (Array.isArray(acc) ? acc : acc.items)[0].id;
    const at = '2026-11-01T10:00:00Z';
    const s = call('POST', '/posts', { content: 'x', social_account_ids: [id], scheduled_at: at, schedule: true }).body;
    expect(call('PATCH', `/posts/${s.id}`, { scheduled_at: '2026-11-02T10:00:00Z' }).body.scheduled_at).toBe('2026-11-02T10:00:00.000Z');
    expect(call('PATCH', `/posts/${s.id}`, { scheduled_at: '2020-01-01T00:00:00Z' }).status).toBe(400);
    const d = call('POST', '/posts', { content: 'x', social_account_ids: [id] }).body;
    expect(call('PATCH', `/posts/${d.id}`, { scheduled_at: at }).status).toBe(400);
  });

  it('answers 409 for a post that is not a draft or scheduled', () => {
    const { call } = make();
    const published = (call('GET', '/posts', undefined).body.items as { id: string; status: string }[]).find((p) => p.status === 'published');
    expect(published).toBeTruthy();
    expect(call('PATCH', `/posts/${published!.id}`, { content: 'nope' }).status).toBe(409);
  });
});
