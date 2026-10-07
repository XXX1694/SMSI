import { describe, expect, it } from 'vitest';
import { charCount, counterTone, effectiveContent, validateComposer, type ComposerState } from '@/lib/composer';
import { normalizeProvider } from '@/lib/normalize';
import type { Media, SocialAccount } from '@/lib/types';

const providers = [
  normalizeProvider({ provider: 'mock', configured: true, capabilities: { can_publish_text: true, can_publish_image: true, max_text_length: 10, max_media_count: 1 } }),
  normalizeProvider({ provider: 'telegram', configured: true, capabilities: { can_publish_text: true, can_publish_image: true, can_publish_video: true, max_text_length: 4096, max_media_count: 10 } }),
];
const acct = (id: string, provider: string, status: SocialAccount['status'] = 'active'): SocialAccount => ({
  id, provider, username: id, display_name: id, avatar_url: null, status, scopes: [], connected_at: '',
});
const accounts = [acct('a1', 'mock'), acct('a2', 'telegram'), acct('a3', 'mock', 'expired')];
const base: ComposerState = { content: 'hello', overrides: {}, accountIds: ['a1'], media: [], scheduledAtUtc: null };
const video: Media = { id: 'v', kind: 'video', mime_type: 'video/mp4', size_bytes: 1, original_name: 'v.mp4', width: null, height: null, status: 'ready', created_at: '' };

describe('validateComposer', () => {
  it('passes for valid content', () => {
    expect(validateComposer(base, accounts, providers)).toEqual([]);
  });

  it('requires an account and content', () => {
    expect(validateComposer({ ...base, accountIds: [] }, accounts, providers)[0]?.message).toMatch(/at least one account/i);
    expect(validateComposer({ ...base, content: '  ' }, accounts, providers)[0]?.message).toMatch(/empty/i);
  });

  it('flags text over the per-platform limit using overrides', () => {
    const s = { ...base, accountIds: ['a1', 'a2'], content: 'x'.repeat(50) };
    const issues = validateComposer(s, accounts, providers);
    expect(issues).toHaveLength(1);
    expect(issues[0]?.accountId).toBe('a1');
    expect(issues[0]?.message).toContain('40 characters over');
    expect(validateComposer({ ...s, overrides: { a1: 'short' } }, accounts, providers)).toEqual([]);
  });

  it('counts code points, not UTF-16 units', () => {
    expect(charCount('😀😀')).toBe(2);
    const issues = validateComposer({ ...base, content: '😀'.repeat(10) }, accounts, providers);
    expect(issues).toEqual([]);
  });

  it('rejects unsupported media and counts', () => {
    const issues = validateComposer({ ...base, media: [video] }, accounts, providers);
    expect(issues.some((i) => /video/i.test(i.message))).toBe(true);
    const two = validateComposer({ ...base, media: [video, { ...video, id: 'v2' }] }, accounts, providers);
    expect(two.some((i) => /at most 1/.test(i.message))).toBe(true);
  });

  it('rejects inactive accounts', () => {
    expect(validateComposer({ ...base, accountIds: ['a3'] }, accounts, providers).some((i) => /expired/.test(i.message))).toBe(true);
  });

  it('validates schedule time', () => {
    const now = new Date('2026-01-01T00:00:00Z');
    expect(validateComposer(base, accounts, providers, { requireSchedule: true, now })[0]?.message).toMatch(/valid date/i);
    expect(validateComposer({ ...base, scheduledAtUtc: '2025-12-31T00:00:00Z' }, accounts, providers, { requireSchedule: true, now })[0]?.message).toMatch(/future/i);
    expect(validateComposer({ ...base, scheduledAtUtc: '2026-01-02T00:00:00Z' }, accounts, providers, { requireSchedule: true, now })).toEqual([]);
  });
});

describe('helpers', () => {
  it('effectiveContent prefers non-blank override', () => {
    expect(effectiveContent({ content: 'a', overrides: { x: 'b' } }, 'x')).toBe('b');
    expect(effectiveContent({ content: 'a', overrides: { x: '  ' } }, 'x')).toBe('a');
  });
  it('counterTone thresholds', () => {
    expect(counterTone(5, 10)).toBe('ok');
    expect(counterTone(10, 10)).toBe('warn');
    expect(counterTone(11, 10)).toBe('over');
    expect(counterTone(999, 0)).toBe('ok');
  });
});
