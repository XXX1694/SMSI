'use client';
import { ConfirmDialog } from '@/components/confirm-dialog';
import { api } from '@/lib/api';
import { useTranslations } from '@/i18n/use-translations';

/**
 * Asks before publishing a failed post again. Shared by the post page and the dashboard so the wording and the
 * error handling (ConfirmDialog shows the API error inline, including approval and quota answers) live in one place.
 */
export function RetryPostDialog({ postId, open, onOpenChange, onRetried }: { postId: string; open: boolean; onOpenChange: (open: boolean) => void; onRetried: () => void }) {
  const t = useTranslations();
  return (
    <ConfirmDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('posts.retryTitle')}
      description={t('posts.retryBody')}
      confirmLabel={t('common.retry')}
      onConfirm={async () => {
        await api.posts.retry(postId);
        onRetried();
      }}
    />
  );
}
