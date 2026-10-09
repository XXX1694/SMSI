import { charCount, counterTone } from '@/lib/composer';
import { cn } from '@/lib/utils';
import { useTranslations } from '@/i18n/use-translations';

export function CharCounter({ text, max, label }: { text: string; max: number; label: string }) {
  const t = useTranslations('composer');
  const len = charCount(text);
  const tone = counterTone(len, max);
  return (
    <span
      className={cn('shrink-0 whitespace-nowrap text-xs tabular-nums', tone === 'ok' && 'text-muted-foreground', tone === 'warn' && 'text-warning', tone === 'over' && 'font-medium text-danger')}
      aria-label={max > 0 ? t('counter', { label, len, max }) : t('counterUnlimited', { label, len })}
    >
      {len}
      {max > 0 ? ` / ${max}` : ''}
    </span>
  );
}
