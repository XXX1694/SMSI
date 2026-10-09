import type { AccountStatus, AttemptStatus, PostStatus, TargetStatus } from './types';

export type Tone = 'neutral' | 'info' | 'success' | 'warning' | 'danger' | 'accent';

export interface StatusView {
  label: string;
  tone: Tone;
}

const POST: Record<PostStatus, StatusView> = {
  // Translator note: "Draft" is a noun (a post status and a filter). The verb ("Agents draft posts") needs its own key.
  draft: { label: 'Draft', tone: 'neutral' },
  scheduled: { label: 'Scheduled', tone: 'accent' },
  publishing: { label: 'Publishing', tone: 'info' },
  published: { label: 'Published', tone: 'success' },
  partially_published: { label: 'Partially published', tone: 'warning' },
  failed: { label: 'Failed', tone: 'danger' },
  cancelled: { label: 'Canceled', tone: 'neutral' },
};

const TARGET: Record<TargetStatus, StatusView> = {
  pending: { label: 'Pending', tone: 'neutral' },
  publishing: { label: 'Publishing', tone: 'info' },
  published: { label: 'Published', tone: 'success' },
  failed: { label: 'Failed', tone: 'danger' },
  cancelled: { label: 'Canceled', tone: 'neutral' },
  needs_review: { label: 'Unconfirmed', tone: 'warning' },
};

const ACCOUNT: Record<AccountStatus, StatusView> = {
  active: { label: 'Active', tone: 'success' },
  expired: { label: 'Needs reconnecting', tone: 'warning' },
  // Translator note: "Revoked" is the status of an API key, an MCP connection or an account whose access was removed.
  revoked: { label: 'Revoked', tone: 'neutral' },
  error: { label: 'Error', tone: 'danger' },
};

const ATTEMPT: Record<AttemptStatus, StatusView> = {
  started: { label: 'Started', tone: 'info' },
  succeeded: { label: 'Succeeded', tone: 'success' },
  failed: { label: 'Failed', tone: 'danger' },
  unknown: { label: 'Unconfirmed', tone: 'warning' },
};

function lookup<K extends string>(map: Record<K, StatusView>, key: string): StatusView {
  return (map as Record<string, StatusView>)[key] ?? { label: key, tone: 'neutral' };
}

export const postStatusView = (s: string): StatusView => lookup(POST, s);
export const targetStatusView = (s: string): StatusView => lookup(TARGET, s);
export const accountStatusView = (s: string): StatusView => lookup(ACCOUNT, s);
export const attemptStatusView = (s: string): StatusView => lookup(ATTEMPT, s);

export const POST_STATUSES: PostStatus[] = [
  'draft',
  'scheduled',
  'publishing',
  'published',
  'partially_published',
  'failed',
  'cancelled',
];

/** Actions allowed by the state machine in the architecture contract. */
export function postActions(status: PostStatus): {
  publish: boolean;
  schedule: boolean;
  cancel: boolean;
  retry: boolean;
  del: boolean;
  edit: boolean;
} {
  return {
    publish: status === 'draft',
    schedule: status === 'draft' || status === 'failed' || status === 'partially_published',
    cancel: status === 'draft' || status === 'scheduled',
    retry: status === 'failed' || status === 'partially_published',
    del: ['draft', 'scheduled', 'failed', 'cancelled', 'published'].includes(status),
    edit: status === 'draft' || status === 'scheduled',
  };
}

/** Why a post cannot be edited, or null when it can. Mirrors `PATCH /posts/{id}` (draft and scheduled only). */
export function editBlockedReason(status: PostStatus): string | null {
  if (postActions(status).edit) return null;
  const view = postStatusView(status).label.toLowerCase();
  if (status === 'publishing') return 'This post is being published right now, so it cannot be changed.';
  if (status === 'published') return 'This post is already published. Published posts cannot be edited here; edit it on the network.';
  if (status === 'cancelled') return 'This post was cancelled and cannot be edited.';
  return `Only drafts and scheduled posts can be edited. This post is ${view}.`;
}
