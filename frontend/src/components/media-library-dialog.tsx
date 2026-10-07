'use client';
import { Film } from 'lucide-react';
import { useCallback, useState } from 'react';
import { ErrorState, LoadingRows } from '@/components/states';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogFooter } from '@/components/ui/dialog';
import { api } from '@/lib/api';
import type { Media } from '@/lib/types';
import { cn } from '@/lib/utils';
import { useAsync } from '@/hooks';

function Body({ selected, onDone }: { selected: Media[]; onDone: (m: Media[]) => void }) {
  const load = useCallback(() => api.media.list(), []);
  const { data, error, loading, reload } = useAsync(load);
  const [picked, setPicked] = useState<Media[]>(selected);

  if (loading) return <LoadingRows rows={2} />;
  if (error || !data) return <ErrorState error={error} onRetry={reload} />;
  if (data.length === 0) return <p className="text-sm text-muted-foreground">Your library is empty. Upload a file first.</p>;
  const toggle = (m: Media) =>
    setPicked((cur) => (cur.some((x) => x.id === m.id) ? cur.filter((x) => x.id !== m.id) : [...cur, m]));

  return (
    <>
      <ul className="grid grid-cols-3 gap-2 sm:grid-cols-4">
        {data.map((m) => {
          const on = picked.some((x) => x.id === m.id);
          return (
            <li key={m.id}>
              <button
                type="button"
                aria-pressed={on}
                onClick={() => toggle(m)}
                className={cn('block aspect-square w-full overflow-hidden rounded-md border-2 bg-muted', on ? 'border-accent' : 'border-transparent')}
              >
                {m.kind === 'image' && m.url ? (
                  <img src={m.url} alt={m.original_name} className="h-full w-full object-cover" />
                ) : (
                  <span className="flex h-full w-full flex-col items-center justify-center gap-1 text-xs text-muted-foreground">
                    <Film className="h-4 w-4" aria-hidden />
                    {m.original_name}
                  </span>
                )}
              </button>
            </li>
          );
        })}
      </ul>
      <DialogFooter>
        <Button onClick={() => onDone(picked)}>Attach {picked.length} selected</Button>
      </DialogFooter>
    </>
  );
}

export function MediaLibraryDialog({
  open,
  onOpenChange,
  selected,
  onSelect,
}: {
  open: boolean;
  onOpenChange: (o: boolean) => void;
  selected: Media[];
  onSelect: (m: Media[]) => void;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent title="Media library" description="Choose files to attach to this post.">
        {open ? (
          <Body
            selected={selected}
            onDone={(m) => {
              onSelect(m);
              onOpenChange(false);
            }}
          />
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
