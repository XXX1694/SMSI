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

  it('flags delete, disconnect, publish now and retry now as irreversible', () => {
    expect(['post.delete', 'social_account.disconnect', 'post.publish', 'post.retry_now'].every(isIrreversible)).toBe(true);
    expect(['post.schedule_soon', 'social_account.connect_token'].some(isIrreversible)).toBe(false);
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
      { label: 'Title', value: 'Launch', long: false },
      { label: 'Text', value: 'Hello', long: false },
      { label: 'Networks', value: 'linkedin, telegram', long: false },
      { label: 'Scheduled for', value: expect.stringContaining('12:02'), long: false },
      { label: 'Instance url', value: 'social.example.com', long: false },
    ]);
    expect(summaryLines(base, 'UTC')).toEqual([]);
  });

  it('shows per-network text and media, and marks long text so the card can offer the full text', () => {
    const long = 'x'.repeat(400);
    const lines = summaryLines(
      { ...base, summary: { content: long, targets: [{ platform: 'telegram', content: 'short one' }], media: { count: 3, images: 2, videos: 1 } } },
      'UTC',
    );
    expect(lines).toEqual([
      { label: 'Text', value: long, long: true },
      { label: 'Text on telegram', value: 'short one', long: false },
      { label: 'Media', value: '2 images, 1 video', long: false },
    ]);
    expect(summaryLines({ ...base, summary: { content: 'a\nb\nc\nd\ne' } }, 'UTC')[0]!.long).toBe(true);
    expect(summaryLines({ ...base, summary: { media: { count: 0, images: 0, videos: 0 } } }, 'UTC')).toEqual([]);
  });

  it('tells two accounts on the same network apart and lists accounts instead of bare networks', () => {
    const lines = summaryLines(
      {
        ...base,
        summary: {
          platforms: ['linkedin', 'linkedin'], accounts: ['linkedin · @alex', 'linkedin · @team'], content: 'base',
          targets: [{ platform: 'linkedin', account: 'linkedin · @alex', content: 'one' }, { platform: 'linkedin', account: 'linkedin · @team', content: 'two' }],
        },
      },
      'UTC',
    );
    expect(lines.map((l) => l.label)).toEqual(['Text', 'Text on linkedin · @alex', 'Text on linkedin · @team', 'Accounts']);
    expect(lines.at(-1)!.value).toBe('linkedin · @alex, linkedin · @team');
  });
});

describe('approval copy', () => {
  it('names the real dangerous actions in plain words', () => {
    expect(actionLabel('post.schedule_soon')).toBe('Schedule at short notice');
    expect(actionLabel('post.publish')).toBe('Publish now');
  });
});
