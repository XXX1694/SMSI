import { providerLabel } from '@/lib/normalize';
import type { Media } from '@/lib/types';

interface PreviewProps {
  author: string;
  text: string;
  media: Media[];
}

function MediaStrip({ media }: { media: Media[] }) {
  const first = media[0];
  if (!first) return null;
  return first.kind === 'image' && first.url ? (
    <img src={first.url} alt="" className="mt-2 max-h-56 w-full rounded-md object-cover" />
  ) : (
    <div className="mt-2 flex h-24 items-center justify-center rounded-md bg-muted text-xs text-muted-foreground">
      {first.kind === 'video' ? 'Video' : 'Image'}: {first.original_name}
      {media.length > 1 ? ` +${media.length - 1}` : ''}
    </div>
  );
}

function Empty() {
  return <span className="text-muted-foreground">Your text appears here.</span>;
}

export function LinkedInPreview({ author, text, media }: PreviewProps) {
  return (
    <div className="rounded-lg border bg-background p-4 text-sm">
      <div className="mb-3 flex items-center gap-2">
        <div className="flex h-9 w-9 items-center justify-center rounded-full bg-muted text-xs font-semibold" aria-hidden>
          {author.slice(0, 1).toUpperCase()}
        </div>
        <div>
          <p className="font-semibold leading-tight">{author}</p>
          <p className="text-xs text-muted-foreground">Just now · Public</p>
        </div>
      </div>
      <p className="whitespace-pre-wrap break-words">{text.trim() ? text : <Empty />}</p>
      <MediaStrip media={media} />
    </div>
  );
}

export function TelegramPreview({ author, text, media }: PreviewProps) {
  return (
    <div className="rounded-lg bg-muted p-4 text-sm">
      <div className="max-w-[28rem] rounded-xl rounded-bl-sm border bg-background p-3">
        <p className="mb-1 text-xs font-semibold text-accent">{author}</p>
        <MediaStrip media={media} />
        <p className="mt-1 whitespace-pre-wrap break-words">{text.trim() ? text : <Empty />}</p>
        <p className="mt-1 text-right text-[11px] text-muted-foreground">now</p>
      </div>
    </div>
  );
}

export function GenericPreview({ provider, author, text, media }: PreviewProps & { provider: string }) {
  return (
    <div className="rounded-lg border bg-background p-4 text-sm">
      <p className="mb-2 text-xs text-muted-foreground">
        {providerLabel(provider)} · {author}
      </p>
      <p className="whitespace-pre-wrap break-words">{text.trim() ? text : <Empty />}</p>
      <MediaStrip media={media} />
    </div>
  );
}

export function PlatformPreview({ provider, ...rest }: PreviewProps & { provider: string }) {
  if (provider === 'linkedin') return <LinkedInPreview {...rest} />;
  if (provider === 'telegram') return <TelegramPreview {...rest} />;
  return <GenericPreview provider={provider} {...rest} />;
}
