import { PSEUDO_LOCALE, type AppLocale } from '@/i18n/locales';
import type { Catalog } from '@/i18n/pseudo';

/** `target` laid over `base`: a key missing from the translation falls back to English. */
export function mergeMessages(base: Catalog, target: Catalog): Catalog {
  const out: Catalog = { ...base };
  for (const [k, v] of Object.entries(target)) {
    const b = out[k];
    if (typeof v === 'string') out[k] = v;
    else out[k] = mergeMessages(typeof b === 'object' && b ? b : {}, v);
  }
  return out;
}

export async function loadMessages(locale: AppLocale, en: Catalog): Promise<Catalog> {
  if (locale === 'en') return en;
  if (locale === PSEUDO_LOCALE) return (await import('@/i18n/pseudo')).pseudoCatalog(en);
  return mergeMessages(en, await (await import('@/i18n/load-locale')).loadLocale(locale));
}
