import { useTranslations } from '@/i18n/use-translations';
export function Sparkline({ values, label }: { values: number[]; label: string }) {
  const t = useTranslations('analytics');
  if (values.length < 2) return <span className="text-xs text-muted-foreground">{t('notEnough')}</span>;
  const w = 120;
  const h = 32;
  const min = Math.min(...values);
  const max = Math.max(...values);
  const span = max - min || 1;
  const pts = values.map((v, i) => `${((i / (values.length - 1)) * w).toFixed(1)},${(h - 2 - ((v - min) / span) * (h - 4)).toFixed(1)}`).join(' ');
  return (
    <svg role="img" aria-label={label} width={w} height={h} viewBox={`0 0 ${w} ${h}`} className="text-accent">
      <polyline points={pts} fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round" strokeLinecap="round" />
    </svg>
  );
}
