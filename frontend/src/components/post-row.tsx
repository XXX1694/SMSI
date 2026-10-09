'use client';
import Link from 'next/link';
import { Button } from '@/components/ui/button';
import { PostStatusBadge } from '@/components/status-badge';
import { usePrefs } from '@/components/prefs-provider';
import { editHref, postHref } from '@/lib/demo/config';
import { postLabel, postPlatforms, postTime } from '@/lib/format';
import { postActions } from '@/lib/status';
import { formatDateTime } from '@/lib/time';
import type { Post } from '@/lib/types';

interface RowProps {
  post: Post;
  /** Shown for failed posts: retries the failed targets. The detail page stays one click away on the row itself. */
  onRetry?: (post: Post) => void;
  retrying?: boolean;
}

export function PostRow({ post, onRetry, retrying = false }: RowProps) {
  const { timezone } = usePrefs();
  return (
    <li className="flex items-center gap-2 pr-1 sm:pr-3">
      <Link
        href={postHref(post.id)}
        className="flex min-w-0 flex-1 flex-wrap items-center justify-between gap-x-4 gap-y-1 px-1 py-3 transition-colors hover:bg-muted/60 sm:px-3"
      >
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-medium">{postLabel(post)}</p>
          <p className="text-xs text-muted-foreground">
            {postPlatforms(post)} · {formatDateTime(postTime(post), timezone)}
          </p>
        </div>
        <PostStatusBadge status={post.status} />
      </Link>
      {postActions(post.status).edit ? (
        <Button asChild size="sm" variant="ghost" className="w-12 shrink-0 px-0">
          <Link href={editHref(post.id)} aria-label={`Edit ${postLabel(post)}`}>
            Edit
          </Link>
        </Button>
      ) : onRetry && postActions(post.status).retry ? (
        <Button size="sm" variant="secondary" className="shrink-0" loading={retrying} onClick={() => onRetry(post)} aria-label={`Retry ${postLabel(post)}`}>
          {retrying ? 'Retrying…' : 'Retry'}
        </Button>
      ) : (
        <span aria-hidden className="w-12 shrink-0 max-sm:hidden" />
      )}
    </li>
  );
}

export function PostList({ posts, onRetry, retryingId }: { posts: Post[]; onRetry?: (post: Post) => void; retryingId?: string | null }) {
  return (
    <ul className="stagger divide-y overflow-hidden rounded-lg border">
      {posts.map((p) => (
        <PostRow key={p.id} post={p} onRetry={onRetry} retrying={retryingId === p.id} />
      ))}
    </ul>
  );
}
