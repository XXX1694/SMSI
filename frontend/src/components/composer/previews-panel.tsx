import { effectiveContent } from '@/lib/composer';
import { providerLabel } from '@/lib/normalize';
import type { Media, SocialAccount } from '@/lib/types';
import { PlatformPreview } from './previews';

export function PreviewsPanel({
  selected,
  content,
  overrides,
  media,
}: {
  selected: SocialAccount[];
  content: string;
  overrides: Record<string, string>;
  media: Media[];
}) {
  return (
    <section aria-label="Live previews" className="space-y-3">
      <h2 className="text-sm font-semibold">Preview</h2>
      {selected.length === 0 ? (
        <p className="rounded-lg border border-dashed px-4 py-6 text-center text-sm text-muted-foreground">
          Select an account to see how the post will look.
        </p>
      ) : (
        selected.map((a) => (
          <div key={a.id} className="space-y-1.5">
            <p className="text-xs text-muted-foreground">{providerLabel(a.provider)} · {a.display_name}</p>
            <PlatformPreview
              provider={a.provider}
              author={a.display_name || a.username}
              text={effectiveContent({ content, overrides }, a.id)}
              media={media}
            />
          </div>
        ))
      )}
    </section>
  );
}
