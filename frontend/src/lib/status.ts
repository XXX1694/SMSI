import type { AppT } from '@/i18n/translate';
import type { AccountStatus, AttemptStatus, PostStatus, TargetStatus } from './types';

export type Tone = 'neutral' | 'info' | 'success' | 'warning' | 'danger' | 'accent';

/** A status as a tag shows it. */
export interface StatusView {
  label: string;
  tone: Tone;
  /** The shape shown with the label (BRAND.md section 5); absent for a status this version does not know. */
  glyph?: GlyphName;
}

/** The shapes a status tag can show (components/ui/status-glyph.tsx draws them). One shape per meaning, so colour is never the only cue. */
export type GlyphName = 'draft' | 'waiting' | 'scheduled' | 'progress' | 'done' | 'partial' | 'failed' | 'alert' | 'off';

type Look = [Tone, GlyphName];

const POST: Record<PostStatus, Look> = {
  draft: ['neutral', 'draft'],
  scheduled: ['accent', 'scheduled'],
  publishing: ['info', 'progress'],
  published: ['success', 'done'],
  partially_published: ['warning', 'partial'],
  failed: ['danger', 'failed'],
  cancelled: ['neutral', 'off'],
};

const TARGET: Record<TargetStatus, Look> = {
  pending: ['neutral', 'scheduled'],
  publishing: ['info', 'progress'],
  published: ['success', 'done'],
  failed: ['danger', 'failed'],
  cancelled: ['neutral', 'off'],
  needs_review: ['warning', 'waiting'],
};

const ACCOUNT: Record<AccountStatus, Look> = {
  active: ['success', 'done'],
  expired: ['warning', 'alert'],
  revoked: ['neutral', 'off'],
  error: ['danger', 'failed'],
};

const ATTEMPT: Record<AttemptStatus, Look> = {
  started: ['info', 'progress'],
  succeeded: ['success', 'done'],
  failed: ['danger', 'failed'],
  unknown: ['warning', 'alert'],
};

/** A status the server sent that this version does not know shows as sent, in the neutral tone. */
function view(looks: Record<string, Look>, kind: 'post' | 'target' | 'account' | 'attempt', status: string, t: AppT): StatusView {
  const look = looks[status];
  if (!look) return { label: status, tone: 'neutral' };
  // The key is built from a known status, so it exists in the catalog.
  return { label: t(`common.status.${kind}.${status}` as 'common.status.post.draft'), tone: look[0], glyph: look[1] };
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
  return t('posts.editBlockedOther', { status });
}
