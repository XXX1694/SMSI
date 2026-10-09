'use client';
import { useTranslations } from '@/i18n/use-translations';
import { ENDONYMS, PSEUDO_LOCALE, isBeta, type AppLocale } from '@/i18n/locales';
import { useLocaleSettings } from '@/i18n/locale-provider';
import { Select } from '@/components/ui/input';
import { cn } from '@/lib/utils';

/** Lists only the available locales, by endonym; machine-drafted ones carry a "Beta translation" label. */
export function LanguageSelect({ compact = false, className }: { compact?: boolean; className?: string }) {
  const t = useTranslations('language');
  const { locale, setLocale, available } = useLocaleSettings();
  const label = (l: AppLocale) =>
    l === PSEUDO_LOCALE ? t('pseudo') : isBeta(l) ? t('optionBeta', { name: ENDONYMS[l] }) : ENDONYMS[l];
  const options = available.map((l) => (
    <option key={l} value={l} lang={l === PSEUDO_LOCALE ? 'en' : l}>
      {label(l)}
    </option>
  ));
  const onChange = (v: string) => setLocale(v as AppLocale);
  if (!compact) {
    return (
      <Select value={locale} onChange={(e) => onChange(e.target.value)} className={className}>
        {options}
      </Select>
    );
  }
  return (
    <select
      aria-label={t('label')}
      value={locale}
      onChange={(e) => onChange(e.target.value)}
      className={cn(
        'h-7 w-full rounded-md border bg-transparent px-1.5 text-xs text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
        className,
      )}
    >
      {options}
    </select>
  );
}
