import { BUNDLES, type BundleId } from '@/i18n/catalog';
import type { Locale } from '@/i18n/locales';
import type { Catalog } from '@/i18n/pseudo';

/**
 * Loads a locale from its bundle files, `messages/{locale}/{bundle}.json`. Each file is a lazy chunk, so catalogs are not
 * part of the shared JS; this module is a lazy chunk too, because it carries the chunk map of every file. English is left
 * out of that map: it arrives with the page (see en-all.ts). A bundle the locale has no file for is skipped and falls back
 * to English in `loadMessages`; so does a locale without any file.
 */
async function loadBundle(locale: Locale, bundle: BundleId): Promise<Catalog | null> {
  try {
    return (await import(/* webpackExclude: /[\\/]en[\\/]/ */ `../../messages/${locale}/${bundle}.json`)).default as Catalog;
  } catch (e) {
    if (isMissingFile(e)) return null;
    throw e; // a chunk that failed to load (offline) is an error: the provider then stays on English
  }
}

/** webpack rejects with code MODULE_NOT_FOUND; Vite (tests) with "Unknown variable dynamic import". */
function isMissingFile(e: unknown): boolean {
  const err = e as { code?: string; message?: string } | null;
  return err?.code === 'MODULE_NOT_FOUND' || /unknown variable dynamic import|cannot find module/i.test(err?.message ?? '');
}

export async function loadLocale(locale: Locale): Promise<Catalog> {
  const parts = await Promise.all(BUNDLES.map((bundle) => loadBundle(locale, bundle)));
  const out: Catalog = {};
  BUNDLES.forEach((bundle, i) => {
    const part = parts[i];
    if (part) out[bundle] = part;
  });
  return out;
}
