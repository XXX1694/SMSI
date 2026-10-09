'use client';
import { Trash2 } from 'lucide-react';
import { useState } from 'react';
import { useAuth } from '@/components/auth-provider';
import { usePrefs } from '@/components/prefs-provider';
import { Button } from '@/components/ui/button';
import { errorMessage } from '@/hooks';
import { api } from '@/lib/api';
import { formatDateTime } from '@/lib/time';

/** Above every screen while the account is scheduled for deletion: when it happens, and the way back. */
export function DeletionBanner() {
  const { user, refresh } = useAuth();
  const { timezone } = usePrefs();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  if (!user?.deletion_scheduled_at) return null;

  async function cancel() {
    setBusy(true);
    setError(null);
    try {
      await api.account.cancelDeletion();
      await refresh();
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div role="alert" className="flex flex-wrap items-center gap-x-4 gap-y-2 border-b border-danger/30 bg-danger-soft px-4 py-2.5 text-sm md:px-10">
      <Trash2 className="hidden h-4 w-4 shrink-0 text-danger sm:block" aria-hidden />
      <p className="min-w-0 flex-1 basis-64">
        <span className="font-medium">Your account will be deleted on {formatDateTime(user.deletion_scheduled_at, timezone)}.</span> All your data goes then and cannot be
        recovered. Your API keys were revoked and scheduled posts are drafts. You can still download your data in Settings.
      </p>
      <Button variant="secondary" size="sm" onClick={() => void cancel()} disabled={busy}>
        {busy ? 'Cancelling…' : 'Cancel deletion'}
      </Button>
      {error ? <span className="w-full text-danger">{error}</span> : null}
    </div>
  );
}
