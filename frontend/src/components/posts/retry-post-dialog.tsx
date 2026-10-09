'use client';
import { ConfirmDialog } from '@/components/confirm-dialog';
import { api } from '@/lib/api';

/**
 * Asks before publishing a failed post again. Shared by the post page and the dashboard so the wording and the
 * error handling (ConfirmDialog shows the API error inline, including approval and quota answers) live in one place.
 */
export function RetryPostDialog({ postId, open, onOpenChange, onRetried }: { postId: string; open: boolean; onOpenChange: (open: boolean) => void; onRetried: () => void }) {
  return (
    <ConfirmDialog
      open={open}
      onOpenChange={onOpenChange}
      title="Retry failed accounts?"
      description="Steerpost publishes again only to the accounts that failed."
      confirmLabel="Retry"
      onConfirm={async () => {
        await api.posts.retry(postId);
        onRetried();
      }}
    />
  );
}
