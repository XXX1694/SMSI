import type { AppT } from '@/i18n/translate';
import type { AccountStatus, AttemptStatus, PostStatus, TargetStatus } from './types';

export type Tone = 'neutral' | 'info' | 'success' | 'warning' | 'danger' | 'accent';

export interface StatusView {
  label: string;
  tone: Tone;
}

const POST: Record<PostStatus, Tone> = {
  draft: 'neutral',
  scheduled: 'accent',
  publishing: 'info',
  published: 'success',
  partially_published: 'warning',
  failed: 'danger',
  cancelled: 'neutral',
};

const TARGET: Record<TargetStatus, Tone> = {
  pending: 'neutral',
  publishing: 'info',
  published: 'success',
  failed: 'danger',
  cancelled: 'neutral',
  needs_review: 'warning',
};

const ACCOUNT: Record<AccountStatus, Tone> = {
  active: 'success',
  expired: 'warning',
  revoked: 'neutral',
  error: 'danger',
};

const ATTEMPT: Record<AttemptStatus, Tone> = {
  started: 'info',
  succeeded: 'success',
  failed: 'danger',
  unknown: 'warning',
};

/** A status the server sent that this version does not know shows as sent, in the neutral tone. */
function view(tones: Record<string, Tone>, kind: 'post' | 'target' | 'account' | 'attempt', status: string, t: AppT): StatusView {
  const tone = tones[status];
  // The key is built from a known status, so it exists in the catalog.
  return tone ? { label: t(`common.status.${kind}.${status}` as 'common.status.post.draft'), tone } : { label: status, tone: 'neutral' };
}

export const postStatusView = (s: string, t: AppT): StatusView => view(POST, 'post', s, t);
export const targetStatusView = (s: string, t: AppT): StatusView => view(TARGET, 'target', s, t);
export const accountStatusView = (s: string, t: AppT): StatusView => view(ACCOUNT, 'account', s, t);
export const attemptStatusView = (s: string, t: AppT): StatusView => view(ATTEMPT, 'attempt', s, t);

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
export function editBlockedReason(status: PostStatus, t: AppT): string | null {
  if (postActions(status).edit) return null;
  if (status === 'publishing') return t('posts.editBlockedPublishing');
  if (status === 'published') return t('posts.editBlockedPublished');
  if (status === 'cancelled') return t('posts.editBlockedCanceled');
  return t('posts.editBlockedOther', { status: postStatusView(status, t).label.toLowerCase() });
}
