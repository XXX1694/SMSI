import { PSEUDO_LOCALE, type AppLocale, type Locale } from '@/i18n/locales';
import type { Catalog } from '@/i18n/pseudo';

/**
 * One lazy chunk per locale (src/i18n/catalogs/{locale}.ts imports that locale's messages/{locale}/*.json), so catalogs are
 * not part of the shared JS and the build's chunk map stays small. English arrives with the page (see en-all.ts). Literal
 * paths, not a template over the directory: webpack would otherwise emit a chunk and a map entry per file.
 */
const LOADERS: Record<Exclude<Locale, 'en'>, () => Promise<{ default: Catalog }>> = {
  ru: () => import('@/i18n/catalogs/ru'),
  es: () => import('@/i18n/catalogs/es'),
  'pt-BR': () => import('@/i18n/catalogs/pt-BR'),
  de: () => import('@/i18n/catalogs/de'),
  fr: () => import('@/i18n/catalogs/fr'),
  id: () => import('@/i18n/catalogs/id'),
  ja: () => import('@/i18n/catalogs/ja'),
  'zh-CN': () => import('@/i18n/catalogs/zh-CN'),
  kk: () => import('@/i18n/catalogs/kk'),
  ar: () => import('@/i18n/catalogs/ar'),
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
