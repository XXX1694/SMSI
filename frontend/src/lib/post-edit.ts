import { utcToZonedInputs } from './time';
import type { ComposerState } from './composer';
import type { Media, Post, UpdatePostInput } from './types';

/** Everything the composer form edits, as plain strings so two snapshots can be compared. */
export interface FormValues {
  title: string;
  content: string;
  overrides: Record<string, string>;
  accountIds: string[];
  media: Media[];
  date: string;
  time: string;
}

export const EMPTY_FORM: FormValues = { title: '', content: '', overrides: {}, accountIds: [], media: [], date: '', time: '09:00' };

/** The form for an existing post: base text, per-network overrides (targets that differ), media and the time in `tz`. */
export function formFromPost(post: Post, tz: string): FormValues {
  const content = post.content ?? post.targets[0]?.content ?? '';
  const overrides: Record<string, string> = {};
  for (const t of post.targets) if (t.content !== content) overrides[t.social_account_id] = t.content;
  const when = post.status === 'scheduled' && post.scheduled_at ? utcToZonedInputs(post.scheduled_at, tz) : null;
  return {
    title: post.title ?? '',
    content,
    overrides,
    accountIds: post.targets.map((t) => t.social_account_id),
    media: post.media ?? [],
    date: when?.date ?? '',
    time: when?.time ?? '09:00',
  };
}

/** True when the user changed anything. Order of accounts and empty overrides do not count. */
export function isDirty(a: FormValues, b: FormValues): boolean {
  return JSON.stringify(canonical(a)) !== JSON.stringify(canonical(b));
}

function canonical(v: FormValues) {
  const overrides = Object.entries(v.overrides)
    .filter(([, text]) => text.trim() !== '')
    .sort(([x], [y]) => x.localeCompare(y));
  return { t: v.title.trim(), c: v.content, o: overrides, a: [...v.accountIds].sort(), m: v.media.map((x) => x.id), d: v.date, h: v.date ? v.time : '' };
}

/**
 * The PATCH body. Every account gets an explicit target text (its override or the base text) so a cleared override is
 * really cleared. `scheduled_at` is sent only for a scheduled post whose date or time the user changed.
 */
export function buildUpdate(
  state: ComposerState,
  form: FormValues,
  initial: FormValues,
  status: Post['status'],
): UpdatePostInput {
  const targets = state.accountIds.map((id) => {
    const o = state.overrides[id]?.trim();
    return { social_account_id: id, content: o ? state.overrides[id] ?? '' : state.content };
  });
  const timeChanged = form.date !== initial.date || form.time !== initial.time;
  return {
    title: form.title.trim(),
    content: state.content,
    social_account_ids: state.accountIds,
    media_ids: state.media.map((m) => m.id),
    targets,
    ...(status === 'scheduled' && timeChanged && state.scheduledAtUtc ? { scheduled_at: state.scheduledAtUtc } : {}),
  };
}

export type Freshness = 'same' | 'changed' | 'unknown';

/** Compares the version the form was loaded from with what the server has now. The API has no ETag, so `updated_at` it is. */
export function freshness(loaded: Post, latest: Post): Freshness {
  if (!loaded.updated_at || !latest.updated_at) return 'unknown';
  return loaded.updated_at === latest.updated_at && loaded.status === latest.status ? 'same' : 'changed';
}
