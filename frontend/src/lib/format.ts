import type { AppT } from '@/i18n/translate';
import { providerLabel } from './normalize';
import type { Post } from './types';

/** Items on one line: "LinkedIn, Telegram". The separator comes from the catalog. */
export function joinList(items: readonly string[], t: AppT): string {
  return items.join(t('common.listSeparator'));
}

export function postLabel(post: Pick<Post, 'title' | 'content' | 'targets'>, t: AppT): string {
  const text = post.title?.trim() || post.content?.trim() || post.targets[0]?.content?.trim() || t('posts.untitled');
  return text.length > 90 ? `${text.slice(0, 89)}…` : text;
}

export function postPlatforms(post: Pick<Post, 'targets'>, t: AppT): string {
  const names = [...new Set(post.targets.map((x) => providerLabel(x.platform)))];
  return joinList(names, t) || t('posts.noTargets');
}

/** The timestamp most relevant to a post given its status. */
export function postTime(post: Pick<Post, 'status' | 'scheduled_at' | 'published_at' | 'created_at'>): string {
  if (post.status === 'published' || post.status === 'partially_published') return post.published_at ?? post.created_at;
  return post.scheduled_at ?? post.created_at;
}
