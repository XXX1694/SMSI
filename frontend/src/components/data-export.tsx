'use client';
import { Download, Loader2 } from 'lucide-react';
import { useCallback, useEffect, useState } from 'react';
import { ErrorState, InlineError, LoadingRows } from '@/components/states';
import { Button } from '@/components/ui/button';
import { useAsync, useErrorText } from '@/hooks';
import { useTranslations } from '@/i18n/use-translations';
import { useFormat } from '@/i18n/use-format';
import { api } from '@/lib/api';
import { formatBytes } from '@/lib/media';
import type { DataExport } from '@/lib/types';

const POLL_MS = 3000;

/** The export the card is about: the newest one. Older entries are expired or failed and would only add noise. */
function newest(list: DataExport[]): DataExport | null {
  return [...list].sort((a, b) => b.created_at.localeCompare(a.created_at))[0] ?? null;
}

function save(url: string): void {
  const a = document.createElement('a');
  a.href = url;
  a.download = url.startsWith('data:') ? 'socialos-export.json' : 'socialos-export.zip';
  a.rel = 'noopener';
  document.body.appendChild(a);
  a.click();
  a.remove();
}

/** Export everything the account holds as a ZIP (D-018). Loading, empty, preparing, ready, failed and expired are all shown. */
export function DataExportCard() {
  const t = useTranslations();
  const fmt = useFormat();
  const errorText = useErrorText();
  const load = useCallback(() => api.account.exports.list(), []);
  const { data, error, loading, reload } = useAsync(load);
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState<string | null>(null);
  const current = data ? newest(data) : null;
  const preparing = current?.status === 'pending' || current?.status === 'running';

  useEffect(() => {
    if (!preparing) return undefined;
    const timer = setInterval(reload, POLL_MS);
    return () => clearInterval(timer);
  }, [preparing, reload]);

  async function start() {
    setBusy(true);
    setProblem(null);
    try {
      await api.account.exports.request();
      reload();
    } catch (e) {
      setProblem(errorText(e));
    } finally {
      setBusy(false);
    }
  }

  async function download(id: string) {
    setBusy(true);
    setProblem(null);
    try {
      save((await api.account.exports.link(id)).url);
    } catch (e) {
      setProblem(errorText(e));
    } finally {
      setBusy(false);
    }
  }

  if (loading && !data) return <LoadingRows rows={1} />;
  if (error || !data) return <ErrorState error={error} onRetry={reload} title={t('settings.export.loadFailed')} />;

  const when = (iso: string | null) => (iso ? fmt.dateTime(iso) : '');
  return (
    <div className="space-y-3">
      <p className="text-sm text-muted-foreground">
        {t('settings.export.intro')}
      </p>
      {preparing ? (
        <p role="status" className="flex items-center gap-2 text-sm">
          <Loader2 className="h-4 w-4 animate-spin" aria-hidden /> {t('settings.export.preparing')}
        </p>
      ) : null}
      {current?.status === 'ready' ? (
        <p className="text-sm">
          {t('settings.export.ready', { size: formatBytes(current.size_bytes, t), date: when(current.expires_at) })}
        </p>
      ) : null}
      {current?.status === 'failed' ? <InlineError>{t('settings.export.failed')}</InlineError> : null}
      {current?.status === 'expired' ? <p className="text-sm text-muted-foreground">{t('settings.export.expired')}</p> : null}
      {problem ? <InlineError>{problem}</InlineError> : null}
      <div className="flex flex-wrap gap-2">
        {current?.status === 'ready' ? (
          <Button onClick={() => download(current.id)} disabled={busy}>
            <Download className="mr-2 h-4 w-4" aria-hidden /> {t('settings.export.download')}
          </Button>
        ) : null}
        {!preparing ? (
          <Button variant={current?.status === 'ready' ? 'secondary' : 'primary'} onClick={start} disabled={busy}>
            {current?.status === 'ready' ? t('settings.export.requestNew') : t('settings.export.request')}
          </Button>
        ) : null}
      </div>
      {current?.status === 'ready' ? <p className="text-xs text-muted-foreground">{t('settings.export.cooldown')}</p> : null}
    </div>
  );
}
