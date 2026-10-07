import { charCount, counterTone } from '@/lib/composer';
import { cn } from '@/lib/utils';

export function CharCounter({ text, max, label }: { text: string; max: number; label: string }) {
  const len = charCount(text);
  const tone = counterTone(len, max);
  return (
    <span
      className={cn('text-xs tabular-nums', tone === 'ok' && 'text-muted-foreground', tone === 'warn' && 'text-warning', tone === 'over' && 'font-medium text-danger')}
      aria-label={`${label}: ${len} of ${max > 0 ? max : 'unlimited'} characters`}
    >
      {len}
      {max > 0 ? ` / ${max}` : ''}
    </span>
  );
}
