import { describe, expect, it } from 'vitest';
import { accountStatusView, attemptStatusView, postActions, postStatusView, targetStatusView } from '@/lib/status';
import { enT } from '@/i18n/en';

describe('state to badge mapping', () => {
  it('maps post statuses', () => {
    expect(postStatusView('draft', enT)).toEqual({ label: 'Draft', tone: 'neutral', glyph: 'draft' });
    expect(postStatusView('scheduled', enT).tone).toBe('accent');
    expect(postStatusView('published', enT).tone).toBe('success');
    expect(postStatusView('partially_published', enT)).toEqual({ label: 'Partially published', tone: 'warning', glyph: 'partial' });
    expect(postStatusView('failed', enT).tone).toBe('danger');
  });
  it('maps target, account and attempt statuses', () => {
    expect(targetStatusView('needs_review', enT).tone).toBe('warning');
    expect(accountStatusView('expired', enT).tone).toBe('warning');
    expect(accountStatusView('expired', enT).label).toBe('Needs reconnecting');
    expect(targetStatusView('needs_review', enT).label).toBe('Unconfirmed');
    expect(attemptStatusView('unknown', enT).label).toBe('Unconfirmed');
  });
  it('gives every known status a glyph, one shape per meaning', () => {
    const post = ['draft', 'scheduled', 'publishing', 'published', 'partially_published', 'failed', 'cancelled'].map((s) => postStatusView(s, enT).glyph);
    expect(post.every(Boolean)).toBe(true);
    expect(new Set(post).size).toBe(post.length);
    for (const [view, list] of [
      [targetStatusView, ['pending', 'publishing', 'published', 'failed', 'cancelled', 'needs_review']],
      [accountStatusView, ['active', 'expired', 'revoked', 'error']],
      [attemptStatusView, ['started', 'succeeded', 'failed', 'unknown']],
    ] as const) {
      const glyphs = list.map((s) => view(s, enT).glyph);
      expect(glyphs.every(Boolean)).toBe(true);
      expect(new Set(glyphs).size).toBe(glyphs.length);
    }
  });
  it('falls back for unknown values', () => {
    expect(postStatusView('weird', enT)).toEqual({ label: 'weird', tone: 'neutral' });
  });
});

describe('postActions follows the state machine', () => {
  it('draft', () => {
    expect(postActions('draft')).toMatchObject({ publish: true, schedule: true, cancel: true, retry: false, del: true, edit: true });
  });
  it('scheduled can be cancelled but not published directly', () => {
    expect(postActions('scheduled')).toMatchObject({ publish: false, cancel: true, retry: false });
  });
  it('failed can be retried and rescheduled', () => {
    expect(postActions('failed')).toMatchObject({ retry: true, schedule: true, cancel: false });
  });
  it('publishing allows nothing; cancelled is terminal but deletable', () => {
    expect(Object.values(postActions('publishing')).every((v) => v === false)).toBe(true);
    expect(postActions('cancelled')).toMatchObject({ publish: false, retry: false, del: true });
  });
});
