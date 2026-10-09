import { ENABLED_LOCALES, type Locale } from '@/i18n/locales';

/** Maps one browser language tag to a locale, `'en'` for tags we deliberately send to English, or null if unknown. */
function mapTag(tag: string): Locale | null {
  const parts = tag.trim().replace(/_/g, '-').split('-');
  const lang = (parts[0] ?? '').toLowerCase();
  const rest = parts.slice(1).map((p) => p.toLowerCase());
  switch (lang) {
    case 'en':
      return 'en';
    case 'uk': // never falls back to ru (docs/copy/languages.md)
      return 'en';
    case 'es':
      return 'es';
    case 'pt':
      return 'pt-BR';
    case 'fr':
      return 'fr';
    case 'de':
      return 'de';
    case 'ru':
      return 'ru';
    case 'id':
    case 'in': // legacy code for Indonesian
      return 'id';
    case 'ja':
      return 'ja';
    case 'kk':
      return 'kk';
    case 'ar':
      return 'ar';
    case 'zh': {
      // Simplified only. zh-TW, zh-HK, zh-MO and zh-Hant get English, never Simplified Chinese.
      if (rest.includes('hant') || rest.some((r) => r === 'tw' || r === 'hk' || r === 'mo')) return 'en';
      return 'zh-CN';
    }
    default:
      return null;
  }
}

/**
 * The best enabled locale for the browser's `navigator.languages` (first tag wins). A recognised tag whose locale is
 * not enabled yet is skipped, so ["ru", "es"] gives `es` while `ru` is not shipped. Everything else gives `en`.
 */
export function matchLocale(tags: readonly string[], enabled: readonly Locale[] = ENABLED_LOCALES): Locale {
  for (const tag of tags) {
    const locale = mapTag(tag);
    if (locale && enabled.includes(locale)) return locale;
    if (locale === 'en') return 'en';
  }
  return 'en';
}
