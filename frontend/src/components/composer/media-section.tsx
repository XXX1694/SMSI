'use client';
import { Film, Paperclip, X } from 'lucide-react';
import { useRef, useState } from 'react';
import { MediaLibraryDialog } from '@/components/media-library-dialog';
import { Button } from '@/components/ui/button';
import { api } from '@/lib/api';
import { ACCEPT_ATTR, validateMediaFile } from '@/lib/media';
import type { Media } from '@/lib/types';
import { errorMessage } from '@/hooks';

export function MediaSection({ media, onChange }: { media: Media[]; onChange: (m: Media[]) => void }) {
  const input = useRef<HTMLInputElement>(null);
  const [errors, setErrors] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [libOpen, setLibOpen] = useState(false);

  async function onFiles(files: FileList | null) {
    if (!files || files.length === 0) return;
    const errs: string[] = [];
    const added: Media[] = [];
    setBusy(true);
    for (const f of Array.from(files)) {
      const problem = validateMediaFile(f);
      if (problem) {
        errs.push(problem);
        continue;
      }
      try {
        added.push(await api.media.upload(f));
      } catch (e) {
        errs.push(`${f.name}: ${errorMessage(e)}`);
      }
    }
    setBusy(false);
    setErrors(errs);
    if (added.length) onChange([...media, ...added]);
    if (input.current) input.current.value = '';
  }

  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-center gap-2">
        <h3 className="mr-2 text-sm font-semibold">Media</h3>
        <input
          ref={input}
          type="file"
          multiple
          accept={ACCEPT_ATTR}
          className="sr-only"
          id="media-upload"
          aria-label="Upload media"
          onChange={(e) => void onFiles(e.target.files)}
        />
        <Button variant="secondary" size="sm" disabled={busy} onClick={() => input.current?.click()}>
          <Paperclip className="h-3.5 w-3.5" aria-hidden />
          {busy ? 'Uploading…' : 'Upload'}
        </Button>
        <Button variant="secondary" size="sm" onClick={() => setLibOpen(true)}>
          From library
        </Button>
        <span className="text-xs text-muted-foreground">JPEG, PNG, WebP, GIF up to 10 MB · MP4, MOV up to 100 MB</span>
      </div>
      {errors.length > 0 ? (
        <ul role="alert" className="space-y-0.5 text-xs text-danger">
          {errors.map((e) => (
            <li key={e}>{e}</li>
          ))}
        </ul>
      ) : null}
      {media.length > 0 ? (
        <ul className="flex flex-wrap gap-2">
          {media.map((m) => (
            <li key={m.id} className="relative h-16 w-16 overflow-hidden rounded-md border bg-muted">
              {m.kind === 'image' && m.url ? (
                <img src={m.url} alt={m.original_name} className="h-full w-full object-cover" />
              ) : (
                <span className="flex h-full w-full items-center justify-center text-muted-foreground">
                  <Film className="h-5 w-5" aria-label={m.original_name} />
                </span>
              )}
              <button
                type="button"
                onClick={() => onChange(media.filter((x) => x.id !== m.id))}
                aria-label={`Remove ${m.original_name}`}
                className="absolute right-0.5 top-0.5 rounded-full bg-background/90 p-0.5"
              >
                <X className="h-3 w-3" aria-hidden />
              </button>
            </li>
          ))}
        </ul>
      ) : null}
      <MediaLibraryDialog open={libOpen} onOpenChange={setLibOpen} selected={media} onSelect={onChange} />
    </div>
  );
}
