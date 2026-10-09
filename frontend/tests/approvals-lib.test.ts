import { describe, expect, it } from 'vitest';
import { actionLabel, isIrreversible, isOpen, summaryLines, timeLeft } from '@/lib/approvals';
import type { Approval } from '@/lib/types';

const NOW = new Date('2026-10-08T12:00:00Z');
const base: Approval = {
  id: 'a1', action: 'post.publish', resource_type: 'post', resource_id: 'p1', actor_label: 'MCP: Claude', status: 'pending',
  summary: {}, created_at: '2026-10-08T11:58:00Z', expires_at: '2026-10-08T12:08:00Z', decided_at: null,
};

describe('approvals helpers', () => {
  it('names every action in plain English and passes unknown ones through', () => {
    expect(actionLabel('post.publish')).toBe('Publish now');
    expect(actionLabel('social_account.connect_token')).toBe('Connect with a token');
    expect(actionLabel('something.new')).toBe('something.new');
  });

  it('flags only delete and disconnect as irreversible', () => {
    expect(['post.delete', 'social_account.disconnect'].every(isIrreversible)).toBe(true);
    expect(['post.publish', 'post.retry_now', 'post.schedule_soon', 'social_account.connect_token'].some(isIrreversible)).toBe(false);
  });

  it('counts down and reports expiry', () => {
    expect(timeLeft('2026-10-08T12:08:00Z', NOW)).toBe('8 min left');
    expect(timeLeft('2026-10-08T12:01:00Z', NOW)).toBe('1 min left');
    expect(timeLeft('2026-10-08T12:00:30Z', NOW)).toBe('Under a minute left');
    expect(timeLeft('2026-10-08T12:00:00Z', NOW)).toBe('Expired');
    expect(timeLeft('garbage', NOW)).toBe('Expired');
  });

  it('is open only while pending and before the deadline', () => {
    expect(isOpen(base, NOW)).toBe(true);
    expect(isOpen({ ...base, status: 'approved' }, NOW)).toBe(false);
    expect(isOpen({ ...base, expires_at: '2026-10-08T11:59:59Z' }, NOW)).toBe(false);
  });

  it('turns the server summary into labelled lines, known fields first, times in the owner timezone', () => {
    const a = {
      ...base,
      summary: { instance_url: 'social.example.com', platforms: ['linkedin', 'telegram'], title: 'Launch', scheduled_at: '2026-10-08T12:02:00Z', content: 'Hello', empty: '' },
    };
    expect(summaryLines(a, 'UTC')).toEqual([
      { label: 'Title', value: 'Launch' },
      { label: 'Text', value: 'Hello' },
      { label: 'Networks', value: 'linkedin, telegram' },
      { label: 'Scheduled for', value: expect.stringContaining('12:02') },
      { label: 'Instance url', value: 'social.example.com' },
    ]);
    expect(summaryLines(base, 'UTC')).toEqual([]);
  });
});
