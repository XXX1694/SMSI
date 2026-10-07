'use client';
import Link from 'next/link';
import { PostStatusBadge } from '@/components/status-badge';
import { usePrefs } from '@/components/prefs-provider';
import { postLabel, postPlatforms, postTime } from '@/lib/format';
import { formatDateTime } from '@/lib/time';
import type { Post } from '@/lib/types';

export function PostRow({ post }: { post: Post }) {
  const { timezone } = usePrefs();
  return (
    <li>
      <Link
        href={`/posts/${post.id}`}
        className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1 px-1 py-3 hover:bg-muted/60 sm:px-3"
      >
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-medium">{postLabel(post)}</p>
          <p className="text-xs text-muted-foreground">
            {postPlatforms(post)} · {formatDateTime(postTime(post), timezone)}
          </p>
        </div>
        <PostStatusBadge status={post.status} />
      </Link>
    </li>
  );
}

export function PostList({ posts }: { posts: Post[] }) {
  return <ul className="divide-y rounded-lg border">{posts.map((p) => <PostRow key={p.id} post={p} />)}</ul>;
}
