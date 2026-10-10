import { describe, expect, it } from 'vitest';
import { buildUpdate, formFromPost, freshness, isDirty } from '@/lib/post-edit';
import { editBlockedReason, postActions } from '@/lib/status';
import type { Post, PostStatus } from '@/lib/types';
import { enT } from './helpers/en-t';

const post = (over: Partial<Post> = {}): Post => ({
  id: 'p1', title: 'Launch', content: 'Base text', status: 'scheduled', scheduled_at: '2026-12-01T22:30:00Z', published_at: null,
  created_by: 'user', created_at: '2026-10-01T00:00:00Z', updated_at: '2026-10-02T00:00:00Z',
  targets: [
    { id: 't1', social_account_id: 'a1', platform: 'telegram', content: 'Base text', status: 'pending', external_url: null, published_at: null, error_code: null, error_message: null, attempt_count: 0 },
    { id: 't2', social_account_id: 'a2', platform: 'mastodon', content: 'Short text', status: 'pending', external_url: null, published_at: null, error_code: null, error_message: null, attempt_count: 0 },
  ],
  ...over,
});

describe('formFromPost', () => {
  it('prefills title, base text, only differing targets as overrides, and the schedule in the given timezone', () => {
    const f = formFromPost(post(), 'Asia/Almaty');
    expect(f.title).toBe('Launch');
    expect(f.content).toBe('Base text');
    expect(f.overrides).toEqual({ a2: 'Short text' });
    expect(f.accountIds).toEqual(['a1', 'a2']);
    expect(f).toMatchObject({ date: '2026-12-02', time: '03:30' });
  });

  it('leaves the schedule empty for a draft and falls back to the first target for the base text', () => {
    const f = formFromPost(post({ status: 'draft', scheduled_at: null, content: undefined, title: null }), 'UTC');
    expect(f).toMatchObject({ title: '', content: 'Base text', date: '', time: '09:00' });
  });
});

describe('isDirty', () => {
  it('ignores account order, blank overrides and title padding, and sees real edits', () => {
    const base = formFromPost(post(), 'UTC');
    expect(isDirty(base, { ...base, accountIds: ['a2', 'a1'], overrides: { ...base.overrides, a1: '  ' }, title: ' Launch ' })).toBe(false);
    expect(isDirty(base, { ...base, content: 'Base text!' })).toBe(true);
    expect(isDirty(base, { ...base, time: '10:00' })).toBe(true);
  });
});

describe('buildUpdate', () => {
  const base = formFromPost(post(), 'UTC');
  const state = (form = base, at: string | null = '2026-12-01T22:30:00Z') => ({ content: form.content, overrides: form.overrides, accountIds: form.accountIds, media: form.media, scheduledAtUtc: at });

  it('sends an explicit text for every account so a cleared override is really cleared', () => {
    const form = { ...base, overrides: {} };
    const body = buildUpdate(state(form), form, base, 'scheduled');
    expect(body.targets).toEqual([{ social_account_id: 'a1', content: 'Base text' }, { social_account_id: 'a2', content: 'Base text' }]);
  });

  it('keeps the time when it was not touched and sends it when it was', () => {
    expect(buildUpdate(state(), base, base, 'scheduled')).not.toHaveProperty('scheduled_at');
    const moved = { ...base, time: '11:00' };
    expect(buildUpdate(state(moved, '2026-12-02T11:00:00Z'), moved, base, 'scheduled').scheduled_at).toBe('2026-12-02T11:00:00Z');
  });

  it('never sends scheduled_at for a draft (the API wants POST /schedule)', () => {
    const draft = { ...base, date: '2026-12-05' };
    expect(buildUpdate(state(draft, '2026-12-05T09:00:00Z'), draft, { ...base, date: '' }, 'draft')).not.toHaveProperty('scheduled_at');
  });
});

describe('freshness', () => {
  it('flags a newer updated_at or a changed status, and is unknown without updated_at', () => {
    expect(freshness(post(), post())).toBe('same');
    expect(freshness(post(), post({ updated_at: '2026-10-03T00:00:00Z' }))).toBe('changed');
    expect(freshness(post(), post({ status: 'publishing' }))).toBe('changed');
    expect(freshness(post({ updated_at: undefined }), post())).toBe('unknown');
  });
});

describe('editing rules', () => {
  it('allows draft and scheduled only, and explains every other status', () => {
    const all: PostStatus[] = ['draft', 'scheduled', 'publishing', 'published', 'partially_published', 'failed', 'cancelled'];
    for (const s of all) {
      const ok = s === 'draft' || s === 'scheduled';
      expect(postActions(s).edit).toBe(ok);
      expect(editBlockedReason(s, enT) === null).toBe(ok);
    }
    expect(editBlockedReason('published', enT)).toMatch(/already published/);
  });
});
