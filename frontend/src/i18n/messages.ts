import { PSEUDO_LOCALE, type AppLocale, type Locale } from '@/i18n/locales';
import type { Catalog } from '@/i18n/pseudo';

/**
 * One lazy chunk per locale (src/i18n/catalogs/{locale}.ts imports that locale's messages/{locale}/*.json), so catalogs are
 * not part of the shared JS and the build's chunk map stays small. A scope takes the namespaces it needs out of that
 * chunk; per-namespace chunks would add one map entry per namespace and locale (see the PR for step 2 of #145). English
 * arrives with the page (see en-all.ts). Literal paths, not a template over the directory: webpack would otherwise emit
 * a chunk and a map entry per file.
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

const localeChunks = new Map<string, Promise<Catalog>>();
const loaded = new Map<string, Promise<Catalog | null>>();

function localeChunk(locale: Exclude<Locale, 'en'>): Promise<Catalog> {
  let chunk = localeChunks.get(locale);
  if (!chunk) {
    chunk = LOADERS[locale]().then((m) => m.default);
    // A failed import is not kept, so the next attempt (the user picks the language again) can succeed.
    chunk.catch(() => localeChunks.delete(locale));
    localeChunks.set(locale, chunk);
  }
  return chunk;
}

function pick(catalog: Catalog, ids: readonly string[]): Catalog {
  const out: Catalog = {};
  for (const id of ids) if (id in catalog) out[id] = catalog[id] as string | Catalog;
  return out;
}

async function fetchBundles(locale: AppLocale, ids: readonly string[], english: Catalog): Promise<Catalog | null> {
  try {
    if (locale === 'en') return {};
    if (locale === PSEUDO_LOCALE) return (await import('@/i18n/pseudo')).pseudoCatalog(pick(english, ids));
    return pick(await localeChunk(locale), ids);
  } catch (error) {
    console.warn(`i18n: could not load ${locale} messages (${ids.join(', ')}); showing English`, error);
    return null;
  }
}

/**
 * The translated namespaces `ids` of `locale`, to merge over their English (`mergeMessages`); `null` when the chunk
 * failed to load (already logged), which callers treat as "show English". Results are cached per locale and namespace
 * list, and the promise is marked settled once it resolves, so React's `use()` reads a preloaded one without suspending.
 * `retry` forgets an earlier failure: a deliberate language switch is a new attempt.
 */
export function loadBundles(locale: AppLocale, ids: readonly string[], english: Catalog, opts: { retry?: boolean } = {}): Promise<Catalog | null> {
  const key = `${locale}|${ids.join(',')}`;
  const cached = loaded.get(key);
  if (cached && !(opts.retry && (cached as SettledPromise).value === null)) return cached;
  const promise: SettledPromise = fetchBundles(locale, ids, english);
  void promise.then((value) => Object.assign(promise, { status: 'fulfilled', value }));
  loaded.set(key, promise);
  return promise;
}

/** React reads `status` and `value` off a thenable it has seen settle; setting them early skips one suspension. */
type SettledPromise = Promise<Catalog | null> & { status?: 'fulfilled'; value?: Catalog | null };
