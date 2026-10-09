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
import { useTranslations } from '@/i18n/use-translations';

/** Compose a new post, or (with `postId`) edit a draft or scheduled one. */
export function ComposerView({ postId }: { postId?: string }) {
  const t = useTranslations();
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
  if (error || !data) return <ErrorState title={postId ? t('composer.loadFailed') : undefined} showRef={!postId} error={error} onRetry={reload} />;
  const { providers, accounts, post } = data;

  if (post) {
    const reason = editBlockedReason(post.status, t);
    if (reason) {
      return (
        <EmptyState
          title={t('composer.cannotEditTitle')}
          action={
            <Button asChild variant="secondary">
              <Link href={postHref(post.id)}>{t('composer.backToPost')}</Link>
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
        title={t('composer.noAccountsTitle')}
        action={
          <Button asChild>
            <Link href="/accounts">{t('common.connectAccount')}</Link>
          </Button>
        }
      >
        {t('composer.noAccountsBody')}
      </EmptyState>
    );
  }
  const edit = post ? { post, values: formFromPost(post, timezone) } : undefined;
  return <ComposerForm key={post ? `${post.id}:${post.updated_at ?? ''}` : 'new'} accounts={accounts} providers={providers} edit={edit} />;
}
