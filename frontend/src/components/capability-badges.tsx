import { Badge } from '@/components/ui/badge';
import type { Capabilities } from '@/lib/types';
import { useTranslations } from '@/i18n/use-translations';

export function CapabilityBadges({ caps }: { caps: Capabilities }) {
  const t = useTranslations('accounts');
  const items = [
    ['text', caps.canPublishText],
    ['image', caps.canPublishImage],
    ['video', caps.canPublishVideo],
    ['schedule', caps.canSchedule],
    ['delete', caps.canDelete],
    ['analytics', caps.canAnalytics],
  ] as const;
  return (
    <ul className="flex flex-wrap gap-1.5" aria-label={t('caps.label')}>
      {items.map(([key, on]) => (
        <li key={key}>
          <Badge tone={on ? 'neutral' : 'outline'} className={on ? 'text-foreground' : 'border-dashed line-through'}>
            <span className="sr-only">{on ? t('caps.supports') : t('caps.no')} </span>
            {t(`caps.${key}`)}
          </Badge>
        </li>
      ))}
      {caps.maxTextLength > 0 ? (
        <li>
          <Badge tone="outline">{t('caps.characters', { count: caps.maxTextLength })}</Badge>
        </li>
      ) : null}
    </ul>
  );
}
