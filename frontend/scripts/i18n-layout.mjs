/**
 * The file layout of the message catalogs: messages/{locale}/{bundle}.json, src/i18n/catalog.ts and the per-locale
 * indexes in src/i18n/catalogs/. Split from i18n-lib.mjs, which holds the per-key rules.
 */
import { existsSync, readdirSync } from 'node:fs';
import { join } from 'node:path';
import { readJson } from './i18n-lib.mjs';

/** Bundle ids of a locale directory (`messages/ru/` -> ['nav', 'posts', ...]); a locale without a directory has none. */
export function listBundles(localeDir) {
  if (!existsSync(localeDir)) return [];
  return readdirSync(localeDir)
    .filter((f) => f.endsWith('.json'))
    .map((f) => f.slice(0, -'.json'.length))
    .sort();
}

/** Entries of a locale directory that are not `{bundle}.json` files (a stray file or folder would be ignored silently). */
export function strayEntries(localeDir) {
  if (!existsSync(localeDir)) return [];
  return readdirSync(localeDir, { withFileTypes: true })
    .filter((f) => !(f.isFile() && f.name.endsWith('.json')))
    .map((f) => f.name)
    .sort();
}

/** Rebuilds a locale tree from its bundle files: `messages/ru/{nav,posts}.json` -> `{ nav: {...}, posts: {...} }`. */
export function readLocale(localeDir) {
  return Object.fromEntries(listBundles(localeDir).map((id) => [id, readJson(join(localeDir, `${id}.json`))]));
}

/** The bundle ids that `src/i18n/catalog.ts` lists in `BUNDLES`, and those its `Messages` type points at. */
export function catalogBundles(source) {
  const list = [...(/export const BUNDLES = \[([^\]]*)\]/.exec(source)?.[1] ?? '').matchAll(/'([^']+)'/g)].map((m) => m[1]);
  const typed = [...source.matchAll(/typeof import\('\.\.\/\.\.\/messages\/en\/([^']+)\.json'\)/g)].map((m) => m[1]);
  return { list, typed };
}

/** The bundle ids a locale index (`src/i18n/catalogs/{locale}.ts`) imports from `messages/{locale}/`. */
export function indexBundles(source, locale) {
  const re = new RegExp(`from '\\.\\./\\.\\./\\.\\./messages/${locale.replace(/[-]/g, '\\-')}/([^']+)\\.json'`, 'g');
  return [...source.matchAll(re)].map((m) => m[1]);
}

/**
 * Checks the file layout against English and `src/i18n/catalog.ts`.
 * @param {{ bundles: Record<string, string[]>, catalog: { list: string[], typed: string[] }, indexes?: Record<string, string[] | null>, stray?: Record<string, string[]> }} input
 * `bundles` maps a locale to the bundle ids it has a file for; `catalog` is the result of `catalogBundles`; `indexes` maps
 * a non-English locale to the ids its `catalogs/{locale}.ts` imports (null: no such module);
 * `stray` maps a locale to the entries in its folder that are not bundle files.
 * @returns {string[]} errors
 */
export function layoutProblems({ bundles, catalog, indexes = {}, stray = {} }) {
  const errors = [];
  for (const [locale, names] of Object.entries(stray)) {
    for (const name of names) errors.push(`messages/${locale}/${name}: only {bundle}.json files belong in a locale folder`);
  }
  for (const [locale, ids] of Object.entries(bundles)) {
    if (locale === 'en') continue;
    const imported = indexes[locale];
    if (!imported) {
      errors.push(`src/i18n/catalogs/${locale}.ts is missing (it imports the files of messages/${locale}/)`);
      continue;
    }
    for (const id of ids) if (!imported.includes(id)) errors.push(`src/i18n/catalogs/${locale}.ts does not import messages/${locale}/${id}.json`);
    for (const id of imported) if (!ids.includes(id)) errors.push(`src/i18n/catalogs/${locale}.ts imports messages/${locale}/${id}.json, which does not exist`);
  }
  const en = bundles.en ?? [];
  for (const [locale, ids] of Object.entries(bundles)) {
    if (locale === 'en') continue;
    for (const id of ids) if (!en.includes(id)) errors.push(`messages/${locale}/${id}.json: English has no ${id}.json`);
  }
  for (const [where, ids] of [['BUNDLES', catalog.list], ['the Messages type', catalog.typed]]) {
    for (const id of ids) if (!en.includes(id)) errors.push(`src/i18n/catalog.ts: ${where} lists ${id}, but messages/en/${id}.json does not exist`);
    for (const id of en) if (!ids.includes(id)) errors.push(`messages/en/${id}.json is not in ${where} (src/i18n/catalog.ts)`);
  }
  return errors;
}
