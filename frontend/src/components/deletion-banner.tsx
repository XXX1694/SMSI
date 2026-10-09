'use client';
import { Trash2 } from 'lucide-react';
import { useState } from 'react';
import { useAuth } from '@/components/auth-provider';
import { Button } from '@/components/ui/button';
import { useErrorText } from '@/hooks';
import { api } from '@/lib/api';
import { useFormat } from '@/i18n/use-format';
import { useTranslations } from '@/i18n/use-translations';

/** Above every screen while the account is scheduled for deletion: when it happens, and the way back. */
export function DeletionBanner() {
  const { user } = useAuth();
  return user?.deletion_scheduled_at ? <Banner scheduledAt={user.deletion_scheduled_at} /> : null;
}

function Banner({ scheduledAt }: { scheduledAt: string }) {
  const { refresh } = useAuth();
  const t = useTranslations();
  const errorText = useErrorText();
  const fmt = useFormat();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function cancel() {
    setBusy(true);
    setError(null);
    try {
      await api.account.cancelDeletion();
      await refresh();
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div role="alert" className="flex flex-wrap items-center gap-x-4 gap-y-2 border-b border-danger/30 bg-danger-soft px-4 py-2.5 text-sm md:px-10">
      <Trash2 className="hidden h-4 w-4 shrink-0 text-danger sm:block" aria-hidden />
      <p className="min-w-0 flex-1 basis-64">
        <span className="font-medium">{t('shell.deletionBanner.heading', { date: fmt.dateTime(scheduledAt) })}</span>{' '}
        {t('shell.deletionBanner.body')}
      </p>
      <Button variant="secondary" size="sm" onClick={() => void cancel()} disabled={busy}>
        {busy ? t('shell.deletionBanner.cancelling') : t('shell.deletionBanner.cancel')}
      </Button>
      {error ? <span className="w-full text-danger">{error}</span> : null}
    </div>
  );
}
