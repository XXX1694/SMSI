'use client';
import { useSearchParams } from 'next/navigation';
import { PostDetail } from '@/components/posts/post-detail';
import { EmptyState } from '@/components/states';
import { useTranslations } from '@/i18n/use-translations';

/** `/posts/view?id=…`: the post detail for static (demo) hosting, where ids are not path segments. */
export function PostByQuery() {
  const t = useTranslations('posts');
  const id = useSearchParams().get('id');
  if (!id) return <EmptyState title={t('noneSelected')}>{t('openFromList')}</EmptyState>;
  return <PostDetail id={id} />;
}
