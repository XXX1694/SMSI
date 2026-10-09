/** Demo stand-in for the approvals endpoints: the same rules as the backend, on plain data. */
import type { Approval } from '../types';
import { seedId, seedPostId } from './ids';

const MIN = 60_000;
const TTL = 10 * MIN;

type Decided = 'approved' | 'denied';
export type Decision = { ok: true; approval: Approval } | { ok: false; status: 404 | 409; code: string; message: string };

/** Two requests waiting and one the owner already denied, so the page is never empty on a first visit. */
export function seedApprovals(nowMs: number): Approval[] {
  const at = (minutesAgo: number): string => new Date(nowMs - minutesAgo * MIN).toISOString();
  const in10 = (minutesAgo: number): string => new Date(nowMs - minutesAgo * MIN + TTL).toISOString();
  return [
    {
      id: seedId('ab000000', 1), action: 'post.publish', resource_type: 'post', resource_id: seedPostId(0),
      actor_label: 'MCP: Claude Desktop', status: 'pending', created_at: at(2), expires_at: in10(2), decided_at: null,
      summary: {
        title: 'Launch day', content: 'We are live. SocialOS now lets your agent draft while you stay in control. '.repeat(8).trim(),
        platforms: ['linkedin', 'telegram'], accounts: ['linkedin · @demo', 'telegram · @demo_channel'], targets: [{ platform: 'telegram', account: 'telegram · @demo_channel', content: 'We are live. Your agent drafts, you decide.' }],
        media: { count: 2, images: 2, videos: 0 }, status: 'draft',
      },
    },
    {
      id: seedId('ab000000', 2), action: 'post.schedule_soon', resource_type: 'post', resource_id: seedPostId(1),
      actor_label: 'MCP: Cursor', status: 'pending', created_at: at(4), expires_at: in10(4), decided_at: null,
      summary: { title: 'Changelog digest', content: 'This week: approvals, Telegram linking and a calmer dashboard.', platforms: ['telegram'], scheduled_at: new Date(nowMs + 2 * MIN).toISOString() },
    },
    {
      id: seedId('ab000000', 3), action: 'social_account.disconnect', resource_type: 'social_account', resource_id: seedId('a0000000', 3),
      actor_label: 'CI publisher', status: 'denied', created_at: at(95), expires_at: in10(95), decided_at: at(92),
      summary: { provider: 'mock', username: 'demo_mock' },
    },
  ];
}

/** Newest first. `pending` keeps only requests that still wait and have not run out of time. */
export function visibleApprovals(list: Approval[], status: 'pending' | 'all', nowMs: number): Approval[] {
  const open = (a: Approval): boolean => a.status === 'pending' && Date.parse(a.expires_at) > nowMs;
  const shown = (status === 'pending' ? list.filter(open) : list).map((a) => (a.status === 'pending' && !open(a) ? { ...a, status: 'expired' as const } : a));
  return shown.sort((a, b) => b.created_at.localeCompare(a.created_at));
}

export function findApproval(list: Approval[], id: string): Approval | undefined {
  return list.find((a) => a.id === id);
}

/** Only a pending, unexpired approval can be decided; anything else is a 409, an unknown id a 404. */
export function decide(list: Approval[], id: string, to: Decided, nowMs: number): Decision {
  const a = findApproval(list, id);
  if (!a) return { ok: false, status: 404, code: 'NOT_FOUND', message: 'approval not found' };
  if (a.status !== 'pending' || Date.parse(a.expires_at) <= nowMs) {
    return { ok: false, status: 409, code: 'CONFLICT', message: 'this approval is no longer pending' };
  }
  a.status = to;
  a.decided_at = new Date(nowMs).toISOString();
  return { ok: true, approval: a };
}
