'use client';
import { Film, Upload } from 'lucide-react';
import { useCallback, useRef, useState } from 'react';
import { ConfirmDialog } from '@/components/confirm-dialog';
import { usePrefs } from '@/components/prefs-provider';
import { EmptyState, ErrorState, LoadingRows, Notice } from '@/components/states';
import { useToast } from '@/components/toast';
import { Button } from '@/components/ui/button';
import { api } from '@/lib/api';
import { ACCEPT_ATTR, formatBytes, validateMediaFile } from '@/lib/media';
import { formatDateTime } from '@/lib/time';
import type { Media } from '@/lib/types';
import { errorMessage, useAsync } from '@/hooks';

function Thumb({ m }: { m: Media }) {
  return m.kind === 'image' && m.url ? (
    <img src={m.url} alt={m.original_name} className="aspect-square w-full object-cover" loading="lazy" />
  ) : (
    <div className="flex aspect-square w-full items-center justify-center bg-muted text-muted-foreground">
      <Film className="h-6 w-6" aria-label={m.kind === 'video' ? 'Video' : 'File'} />
    </div>
  );
}

export function MediaView() {
  const load = useCallback(() => api.media.list(), []);
  const { data, error, loading, reload } = useAsync(load);
  const { timezone } = usePrefs();
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
      const p = validateMediaFile(f);
      if (p) {
        errs.push(p);
        continue;
      }
      try {
        await api.media.upload(f);
      } catch (e) {
        errs.push(`${f.name}: ${errorMessage(e)}`);
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
        <input ref={input} id="media-lib-upload" type="file" multiple accept={ACCEPT_ATTR} className="sr-only" aria-label="Upload files" onChange={(e) => void onFiles(e.target.files)} />
        <Button onClick={() => input.current?.click()} disabled={busy}>
          <Upload className="h-4 w-4" aria-hidden />
          {busy ? 'Uploading…' : 'Upload'}
        </Button>
        <span className="text-xs text-muted-foreground">Images up to 10 MB · videos up to 100 MB</span>
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
        <EmptyState title="No media yet">Upload images or videos to reuse them across posts.</EmptyState>
      ) : (
        <ul className="stagger grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-4">
          {data.map((m) => (
            <li key={m.id} className="card-lift overflow-hidden rounded-lg border bg-background">
              <Thumb m={m} />
              <div className="space-y-1 p-3">
                <p className="truncate text-sm font-medium" title={m.original_name}>{m.original_name}</p>
                <p className="text-xs text-muted-foreground">
                  {formatBytes(m.size_bytes)} · {formatDateTime(m.created_at, timezone)}
                </p>
                <Button variant="ghost" size="sm" className="-ml-3" onClick={() => setTarget(m)} aria-label={`Delete ${m.original_name}`}>
                  Delete
                </Button>
              </div>
            </li>
          ))}
        </ul>
      )}
      <ConfirmDialog
        open={target !== null}
        onOpenChange={(o) => !o && setTarget(null)}
        title="Delete this file?"
        description={`${target?.original_name ?? 'The file'} will be removed from your library. Drafts using it may lose the attachment.`}
        confirmLabel="Delete"
        destructive
        onConfirm={async () => {
          if (!target) return;
          await api.media.remove(target.id);
          toast.success('File deleted');
          reload();
        }}
      />
    </div>
  );
}
