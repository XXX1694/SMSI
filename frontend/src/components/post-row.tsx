'use client';
import Link from 'next/link';
import { Button } from '@/components/ui/button';
import { PostStatusBadge } from '@/components/status-badge';
import { editHref, postHref } from '@/lib/demo/config';
import { postLabel, postPlatforms, postTime } from '@/lib/format';
import { postActions } from '@/lib/status';
import type { Post } from '@/lib/types';
import { useTranslations } from '@/i18n/use-translations';
import { useFormat } from '@/i18n/use-format';

interface RowProps {
  post: Post;
  /** Shown for failed posts: asks the caller to retry (the caller confirms first). The detail page stays one click away on the row. */
  onRetry?: (post: Post) => void;
}

export function PostRow({ post, onRetry }: RowProps) {
  const t = useTranslations();
  const fmt = useFormat();
  const title = postLabel(post, t);
  return (
    <li className="flex min-w-0 flex-wrap items-center gap-2 pr-1 sm:flex-nowrap sm:pr-3">
      <Link
        href={postHref(post.id)}
        className="flex min-w-0 flex-1 basis-48 flex-wrap items-center justify-between gap-x-4 gap-y-1 px-1 py-3 transition-colors hover:bg-muted/60 sm:px-3"
      >
        <div className="min-w-32 flex-1">
          <p className="truncate text-sm font-medium">{title}</p>
          <p className="text-xs text-muted-foreground">
            {postPlatforms(post, t)} · <span className="whitespace-nowrap">{fmt.dateTime(postTime(post))}</span>
          </p>
        </div>
        <PostStatusBadge status={post.status} />
      </Link>
      {postActions(post.status).edit ? (
        <Button asChild size="sm" variant="ghost" className="ml-auto min-w-12 shrink-0 px-2">
          <Link href={editHref(post.id)} aria-label={t('posts.editLabel', { title })}>
            {t('common.edit')}
          </Link>
        </Button>
      ) : onRetry && postActions(post.status).retry ? (
        <Button size="sm" variant="secondary" className="ml-auto shrink-0" onClick={() => onRetry(post)} aria-label={t('posts.retryLabel', { title })}>
          {t('common.retry')}
        </Button>
      ) : (
        <span aria-hidden className="w-12 shrink-0 max-sm:hidden" />
      )}
    </li>
  );
}

export function PostList({ posts, onRetry }: { posts: Post[]; onRetry?: (post: Post) => void }) {
  return (
    <ul className="divide-y overflow-hidden rounded-lg border bg-background">
      {posts.map((p) => (
        <PostRow key={p.id} post={p} onRetry={onRetry} />
      ))}
    </ul>
  );
}
