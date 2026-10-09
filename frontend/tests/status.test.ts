import { describe, expect, it } from 'vitest';
import {
  ACCOUNT_LOOK,
  APPROVAL_LOOK,
  ATTEMPT_LOOK,
  POST_LOOK,
  TARGET_LOOK,
  accountStatusView,
  attemptStatusView,
  postActions,
  postStatusView,
  targetStatusView,
} from '@/lib/status';
import { enT } from './helpers/en-t';

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
  it('gives every status a glyph, one shape per meaning within each set', () => {
    for (const [name, looks] of Object.entries({ POST_LOOK, TARGET_LOOK, ACCOUNT_LOOK, ATTEMPT_LOOK, APPROVAL_LOOK })) {
      const glyphs = Object.values(looks).map(([, glyph]) => glyph);
      expect(glyphs.every(Boolean), name).toBe(true);
      expect(new Set(glyphs).size, name).toBe(glyphs.length);
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
