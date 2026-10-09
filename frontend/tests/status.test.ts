import { describe, expect, it } from 'vitest';
import { accountStatusView, attemptStatusView, postActions, postStatusView, targetStatusView } from '@/lib/status';

describe('state to badge mapping', () => {
  it('maps post statuses', () => {
    expect(postStatusView('draft')).toEqual({ label: 'Draft', tone: 'neutral' });
    expect(postStatusView('scheduled').tone).toBe('accent');
    expect(postStatusView('published').tone).toBe('success');
    expect(postStatusView('partially_published')).toEqual({ label: 'Partially published', tone: 'warning' });
    expect(postStatusView('failed').tone).toBe('danger');
  });
  it('maps target, account and attempt statuses', () => {
    expect(targetStatusView('needs_review').tone).toBe('warning');
    expect(accountStatusView('expired').tone).toBe('warning');
    expect(accountStatusView('expired').label).toBe('Needs reconnecting');
    expect(targetStatusView('needs_review').label).toBe('Unconfirmed');
    expect(attemptStatusView('unknown').label).toBe('Unconfirmed');
  });
  it('falls back for unknown values', () => {
    expect(postStatusView('weird')).toEqual({ label: 'weird', tone: 'neutral' });
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
