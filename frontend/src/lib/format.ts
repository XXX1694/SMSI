import { providerLabel } from './normalize';
import type { Post } from './types';

export function postLabel(post: Pick<Post, 'title' | 'content' | 'targets'>): string {
  const text = post.title?.trim() || post.content?.trim() || post.targets[0]?.content?.trim() || 'Untitled post';
  return text.length > 90 ? `${text.slice(0, 89)}…` : text;
}

export function postPlatforms(post: Pick<Post, 'targets'>): string {
  const names = [...new Set(post.targets.map((t) => providerLabel(t.platform)))];
  return names.join(', ') || 'No targets';
}

/** The timestamp most relevant to a post given its status. */
export function postTime(post: Pick<Post, 'status' | 'scheduled_at' | 'published_at' | 'created_at'>): string {
  if (post.status === 'published' || post.status === 'partially_published') return post.published_at ?? post.created_at;
  return post.scheduled_at ?? post.created_at;
}
