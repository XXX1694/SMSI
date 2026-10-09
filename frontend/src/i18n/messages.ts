import { PSEUDO_LOCALE, type AppLocale, type Locale } from '@/i18n/locales';
import type { Catalog } from '@/i18n/pseudo';

/** One lazy chunk per locale: catalogs are not part of the shared JS. English arrives with the page (see layout.tsx). */
const LOADERS: Record<Locale, () => Promise<{ default: Catalog }>> = {
  en: () => import('../../messages/en.json'),
  ru: () => import('../../messages/ru.json'),
  es: () => import('../../messages/es.json'),
  'pt-BR': () => import('../../messages/pt-BR.json'),
  de: () => import('../../messages/de.json'),
  fr: () => import('../../messages/fr.json'),
  id: () => import('../../messages/id.json'),
  ja: () => import('../../messages/ja.json'),
  'zh-CN': () => import('../../messages/zh-CN.json'),
  kk: () => import('../../messages/kk.json'),
  ar: () => import('../../messages/ar.json'),
};

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
  return mergeMessages(en, (await LOADERS[locale]()).default);
}
