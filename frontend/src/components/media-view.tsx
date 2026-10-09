'use client';
import { Film, Upload } from 'lucide-react';
import { useCallback, useRef, useState } from 'react';
import { ConfirmDialog } from '@/components/confirm-dialog';
import { EmptyState, ErrorState, LoadingRows, Notice } from '@/components/states';
import { useToast } from '@/components/toast';
import { Button } from '@/components/ui/button';
import { api } from '@/lib/api';
import { ACCEPT_ATTR, formatBytes, validateMediaFile } from '@/lib/media';
import type { Media } from '@/lib/types';
import { useErrorText, useAsync } from '@/hooks';
import { useTranslations } from '@/i18n/use-translations';
import { useFormat } from '@/i18n/use-format';

function Thumb({ m }: { m: Media }) {
  const t = useTranslations();
  return m.kind === 'image' && m.url ? (
    <img src={m.url} alt={m.original_name} className="aspect-square w-full object-cover" loading="lazy" />
  ) : (
    <div className="flex aspect-square w-full items-center justify-center bg-muted text-muted-foreground">
      <Film className="h-6 w-6" aria-label={t(m.kind === 'video' ? 'media.kindVideo' : 'media.kindFile')} />
    </div>
  );
}

export function MediaView() {
  const load = useCallback(() => api.media.list(), []);
  const { data, error, loading, reload } = useAsync(load);
  const t = useTranslations();
  const fmt = useFormat();
  const errorText = useErrorText();
  const toast = useToast();
  const input = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState(false);
  const [problems, setProblems] = useState<string[]>([]);
  const [target, setTarget] = useState<Media | null>(null);

  async function onFiles(files: FileList | null) {
    if (!files?.length) return;
    const errs: string[] = [];
    setBusy(true);
    for (const f of Array.from(files)) {
      const p = validateMediaFile(f, t);
      if (p) {
        errs.push(p);
        continue;
      }
      try {
        await api.media.upload(f);
      } catch (e) {
        errs.push(t('composer.uploadFailed', { name: f.name, reason: errorText(e) }));
      }
    }
    setBusy(false);
    setProblems(errs);
    if (input.current) input.current.value = '';
    reload();
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center gap-3">
        <input ref={input} id="media-lib-upload" type="file" multiple accept={ACCEPT_ATTR} className="sr-only" aria-label={t('media.uploadFiles')} onChange={(e) => void onFiles(e.target.files)} />
        <Button onClick={() => input.current?.click()} disabled={busy}>
          <Upload className="h-4 w-4" aria-hidden />
          {busy ? t('common.uploading') : t('common.upload')}
        </Button>
        <span className="text-xs text-muted-foreground">{t('media.limits')}</span>
      </div>
      {problems.length > 0 ? (
        <Notice tone="danger">
          <ul>{problems.map((p) => <li key={p}>{p}</li>)}</ul>
        </Notice>
      ) : null}
      {loading && !data ? (
        <LoadingRows rows={2} />
      ) : error || !data ? (
        <ErrorState error={error} onRetry={reload} />
      ) : data.length === 0 ? (
        <EmptyState title={t('media.noneTitle')}>{t('media.noneBody')}</EmptyState>
      ) : (
        <ul className="stagger grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-4">
          {data.map((m) => (
            <li key={m.id} className="card-lift overflow-hidden rounded-lg border bg-background">
              <Thumb m={m} />
              <div className="space-y-1 p-3">
                <p className="truncate text-sm font-medium" title={m.original_name}>{m.original_name}</p>
                <p className="text-xs text-muted-foreground">
                  {formatBytes(m.size_bytes, t)} · {fmt.dateTime(m.created_at)}
                </p>
                <Button variant="ghost" size="sm" className="-ml-3" onClick={() => setTarget(m)} aria-label={t('media.deleteLabel', { name: m.original_name })}>
                  {t('common.delete')}
                </Button>
              </div>
            </li>
          ))}
        </ul>
      )}
      <ConfirmDialog
        open={target !== null}
        onOpenChange={(o) => !o && setTarget(null)}
        title={t('media.deleteTitle')}
        description={target ? t('media.deleteBody', { name: target.original_name }) : t('media.deleteBodyUnnamed')}
        confirmLabel={t('common.delete')}
        destructive
        onConfirm={async () => {
          if (!target) return;
          await api.media.remove(target.id);
          toast.success(t('media.deleted'));
          reload();
        }}
      />
    </div>
  );
}
