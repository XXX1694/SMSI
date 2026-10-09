'use client';
import { useSearchParams } from 'next/navigation';
import { ComposerView } from '@/components/composer/composer-view';
import { PageHeader } from '@/components/states';
import { useTranslations } from '@/i18n/use-translations';

/** `/compose` writes a new post; `/compose?post=<id>` edits a draft or scheduled one. */
export function ComposeRoute() {
  const postId = useSearchParams().get('post') || undefined;
  const t = useTranslations('composer');
  return (
    <>
      <PageHeader title={postId ? t('editTitle') : t('title')} description={postId ? t('editSubtitle') : t('subtitle')} />
      <ComposerView postId={postId} />
    </>
  );
}
