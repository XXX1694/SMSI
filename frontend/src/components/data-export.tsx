'use client';
import { Download, Loader2 } from 'lucide-react';
import { useCallback, useEffect, useState } from 'react';
import { ErrorState, InlineError, LoadingRows } from '@/components/states';
import { usePrefs } from '@/components/prefs-provider';
import { Button } from '@/components/ui/button';
import { errorMessage, useAsync } from '@/hooks';
import { api } from '@/lib/api';
import { formatBytes } from '@/lib/media';
import { formatDateTime } from '@/lib/time';
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
  const { timezone } = usePrefs();
  const load = useCallback(() => api.account.exports.list(), []);
  const { data, error, loading, reload } = useAsync(load);
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState<string | null>(null);
  const current = data ? newest(data) : null;
  const preparing = current?.status === 'pending' || current?.status === 'running';

  useEffect(() => {
    if (!preparing) return undefined;
    const t = setInterval(reload, POLL_MS);
    return () => clearInterval(t);
  }, [preparing, reload]);

  async function start() {
    setBusy(true);
    setProblem(null);
    try {
      await api.account.exports.request();
      reload();
    } catch (e) {
      setProblem(errorMessage(e));
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
      setProblem(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }

  if (loading && !data) return <LoadingRows rows={1} />;
  if (error || !data) return <ErrorState error={error} onRetry={reload} title="Could not load your exports" />;

  const when = (iso: string | null) => (iso ? formatDateTime(iso, timezone) : '');
  return (
    <div className="space-y-3">
      <p className="text-sm text-muted-foreground">
        Download a ZIP with your profile, posts with their targets and publishing attempts, connected accounts (without credentials), API key names, the audit log, approvals and your media files. Passwords, keys and tokens are never included.
      </p>
      {preparing ? (
        <p role="status" className="flex items-center gap-2 text-sm">
          <Loader2 className="h-4 w-4 animate-spin" aria-hidden /> Preparing your export. You can leave this page; it is ready when you come back.
        </p>
      ) : null}
      {current?.status === 'ready' ? (
        <p className="text-sm">
          Your export ({formatBytes(current.size_bytes)}) is ready. The download link works for 5 minutes; the file is deleted on {when(current.expires_at)}.
        </p>
      ) : null}
      {current?.status === 'failed' ? <InlineError>The last export could not be built. Nothing was kept; try again.</InlineError> : null}
      {current?.status === 'expired' ? <p className="text-sm text-muted-foreground">Your last export expired and was deleted. Request a new one.</p> : null}
      {problem ? <InlineError>{problem}</InlineError> : null}
      <div className="flex flex-wrap gap-2">
        {current?.status === 'ready' ? (
          <Button onClick={() => download(current.id)} disabled={busy}>
            <Download className="mr-2 h-4 w-4" aria-hidden /> Download ZIP
          </Button>
        ) : null}
        {!preparing ? (
          <Button variant={current?.status === 'ready' ? 'secondary' : 'primary'} onClick={start} disabled={busy}>
            {current?.status === 'ready' ? 'Request a new export' : 'Request export'}
          </Button>
        ) : null}
      </div>
      {current?.status === 'ready' ? <p className="text-xs text-muted-foreground">A new export can be requested 24 hours after the last one.</p> : null}
    </div>
  );
}
