import { describe, expect, it } from 'vitest';
import { actionLabel, isIrreversible, isOpen, summaryLines, timeLeft } from '@/lib/approvals';
import type { Approval } from '@/lib/types';
import { enT } from './helpers/en-t';

const NOW = new Date('2026-10-08T12:00:00Z');
const base: Approval = {
  id: 'a1', action: 'post.publish', resource_type: 'post', resource_id: 'p1', actor_label: 'MCP: Claude', status: 'pending',
  summary: {}, created_at: '2026-10-08T11:58:00Z', expires_at: '2026-10-08T12:08:00Z', decided_at: null,
};

describe('approvals helpers', () => {
  it('names every action in plain English and passes unknown ones through', () => {
    expect(actionLabel('post.publish', enT)).toBe('Publish now');
    expect(actionLabel('social_account.connect_token', enT)).toBe('Connect with a token');
    expect(actionLabel('something.new', enT)).toBe('something.new');
  });

  it('flags delete, disconnect, publish now and retry now as irreversible', () => {
    expect(['post.delete', 'social_account.disconnect', 'post.publish', 'post.retry_now'].every(isIrreversible)).toBe(true);
    expect(['post.schedule_soon', 'social_account.connect_token'].some(isIrreversible)).toBe(false);
  });

  it('counts down and reports expiry', () => {
    expect(timeLeft('2026-10-08T12:08:00Z', enT, NOW)).toBe('8 min left');
    expect(timeLeft('2026-10-08T12:01:00Z', enT, NOW)).toBe('1 min left');
    expect(timeLeft('2026-10-08T12:00:30Z', enT, NOW)).toBe('Under a minute left');
    expect(timeLeft('2026-10-08T12:00:00Z', enT, NOW)).toBe('Expired');
    expect(timeLeft('garbage', enT, NOW)).toBe('Expired');
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
    expect(summaryLines(a, 'UTC', enT)).toEqual([
      { label: 'Title', value: 'Launch', long: false },
      { label: 'Text', value: 'Hello', long: false },
      { label: 'Networks', value: 'LinkedIn, Telegram', long: false },
      { label: 'Scheduled for', value: expect.stringContaining('12:02'), long: false },
      { label: 'Instance url', value: 'social.example.com', long: false },
    ]);
    expect(summaryLines(base, 'UTC', enT)).toEqual([]);
  });

  it('shows per-network text and media, and marks long text so the card can offer the full text', () => {
    const long = 'x'.repeat(400);
    const lines = summaryLines(
      { ...base, summary: { content: long, targets: [{ platform: 'telegram', content: 'short one' }], media: { count: 3, images: 2, videos: 1 } } },
      'UTC',
      enT,
    );
    expect(lines).toEqual([
      { label: 'Text', value: long, long: true },
      { label: 'Text on Telegram', value: 'short one', long: false },
      { label: 'Media', value: '2 images, 1 video', long: false },
    ]);
    expect(summaryLines({ ...base, summary: { content: 'a\nb\nc\nd\ne' } }, 'UTC', enT)[0]!.long).toBe(true);
    expect(summaryLines({ ...base, summary: { media: { count: 0, images: 0, videos: 0 } } }, 'UTC', enT)).toEqual([]);
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
      enT,
    );
    expect(lines.map((l) => l.label)).toEqual(['Text', 'Text on LinkedIn · @alex', 'Text on LinkedIn · @team', 'Accounts']);
    expect(lines.at(-1)!.value).toBe('LinkedIn · @alex, LinkedIn · @team');
  });
});

describe('approval copy', () => {
  it('names the real dangerous actions in plain words', () => {
    expect(actionLabel('post.schedule_soon', enT)).toBe('Schedule in the next few minutes');
    expect(actionLabel('post.publish', enT)).toBe('Publish now');
  });
});

describe('summary shows network names, not ids', () => {
  it('maps platform ids in text lines, networks and accounts', () => {
    const a = {
      summary: { platforms: ['telegram'], accounts: ['linkedin · @demo', 'telegram · @chan'], targets: [{ platform: 'telegram', content: 'Hi' }] },
    } as unknown as Parameters<typeof summaryLines>[0];
    const lines = summaryLines(a, 'UTC', enT);
    expect(lines.find((l) => l.label === 'Accounts')?.value).toBe('LinkedIn · @demo, Telegram · @chan');
    expect(lines.find((l) => l.label === 'Text on Telegram')?.value).toBe('Hi');
  });
});

describe('post status in a summary', () => {
  it('shows a status code as the badge text', () => {
    const a = { summary: { status: 'draft', provider: 'mock' } } as unknown as Parameters<typeof summaryLines>[0];
    expect(summaryLines(a, 'UTC', enT)).toEqual([
      { label: 'Network', value: 'Test network', long: false },
      { label: 'Status', value: 'Draft', long: false },
    ]);
  });
});
