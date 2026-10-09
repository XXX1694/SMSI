'use client';
import Link from 'next/link';
import { useCallback } from 'react';
import { usePrefs } from '@/components/prefs-provider';
import { EmptyState, ErrorState, LoadingRows } from '@/components/states';
import { Button } from '@/components/ui/button';
import { api } from '@/lib/api';
import { postHref } from '@/lib/demo/config';
import { formFromPost } from '@/lib/post-edit';
import { editBlockedReason } from '@/lib/status';
import { useAsync } from '@/hooks';
import { ComposerForm } from './composer-form';

/** Compose a new post, or (with `postId`) edit a draft or scheduled one. */
export function ComposerView({ postId }: { postId?: string }) {
  const { timezone } = usePrefs();
  const load = useCallback(async () => {
    const [providers, accounts, post] = await Promise.all([
      api.social.providers(),
      api.social.accounts(),
      postId ? api.posts.get(postId) : Promise.resolve(null),
    ]);
    return { providers, accounts, post };
  }, [postId]);
  const { data, error, loading, reload } = useAsync(load);

  if (loading && !data) return <LoadingRows rows={4} />;
  if (error || !data) return <ErrorState title={postId ? 'Could not load this post' : undefined} showRef={!postId} error={error} onRetry={reload} />;
  const { providers, accounts, post } = data;

  if (post) {
    const reason = editBlockedReason(post.status);
    if (reason) {
      return (
        <EmptyState
          title="This post cannot be edited"
          action={
            <Button asChild variant="secondary">
              <Link href={postHref(post.id)}>Back to the post</Link>
            </Button>
          }
        >
          {reason}
        </EmptyState>
      );
    }
  }

  if (accounts.length === 0) {
    return (
      <EmptyState
        title="No accounts connected"
        action={
          <Button asChild>
            <Link href="/accounts">Connect an account</Link>
          </Button>
        }
      >
        Connect at least one account before composing a post.
      </EmptyState>
    );
  }
  const edit = post ? { post, values: formFromPost(post, timezone) } : undefined;
  return <ComposerForm key={post ? `${post.id}:${post.updated_at ?? ''}` : 'new'} accounts={accounts} providers={providers} edit={edit} />;
}
