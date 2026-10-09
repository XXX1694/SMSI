/** Demo stand-in for the plan limits (D-014): the backend's "free" defaults, counted from the demo's own data. */
import type { UsageReport } from '../types';
import type { DemoState } from './model';

export const DEMO_LIMITS = { accounts: 5, postsPerMonth: 60, mediaBytes: 500 * 1024 * 1024, agentRpm: 120 } as const;

/** A post counts once it was scheduled or published, and unscheduling does not give the slot back (as in the API). Seeded posts carry no flag, so their status decides. */
function counted(p: { status: string; quota_counted?: boolean }): boolean {
  return p.quota_counted ?? (p.status !== 'draft' && p.status !== 'cancelled');
}

export function demoUsage(s: DemoState, nowMs: number): UsageReport {
  const d = new Date(nowMs);
  const start = new Date(Date.UTC(d.getUTCFullYear(), d.getUTCMonth(), 1));
  const end = new Date(Date.UTC(d.getUTCFullYear(), d.getUTCMonth() + 1, 1));
  return {
    plan: 'free',
    period_start: start.toISOString(),
    period_end: end.toISOString(),
    quotas: {
      connected_accounts: { used: s.accounts.length, limit: DEMO_LIMITS.accounts },
      scheduled_posts_month: { used: s.posts.filter(counted).length, limit: DEMO_LIMITS.postsPerMonth },
      media_bytes: { used: s.media.reduce((n, m) => n + m.size_bytes, 0), limit: DEMO_LIMITS.mediaBytes },
      agent_requests_per_minute: { limit: DEMO_LIMITS.agentRpm },
    },
  };
}

/** The refusal the backend would send, or null when `delta` more of `metric` still fits. */
export function demoQuotaError(s: DemoState, metric: 'connected_accounts' | 'scheduled_posts_month' | 'media_bytes', delta: number, nowMs: number): string | null {
  const q = demoUsage(s, nowMs).quotas[metric];
  if (q.limit < 0 || (q.used ?? 0) + delta <= q.limit) return null;
  if (metric === 'connected_accounts') return `connected accounts limit reached (${q.used} of ${q.limit} used). Disconnect an account to connect another.`;
  if (metric === 'scheduled_posts_month') return `monthly post limit reached (${q.used} of ${q.limit} used). The count restarts on the first day of next month (UTC).`;
  return `storage limit reached (${Math.floor((q.used ?? 0) / 1048576)} MB used of ${Math.floor(q.limit / 1048576)} MB). Delete media you no longer need.`;
}
