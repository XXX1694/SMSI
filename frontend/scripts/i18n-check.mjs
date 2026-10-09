#!/usr/bin/env node
/**
 * `npm run i18n:check` (part of `npm run lint`). Offline, a few seconds.
 *   errors:   a bundle file that English lacks, `src/i18n/catalog.ts` out of step with messages/en/, a catalog key that
 *             English lacks, invalid ICU, placeholders that differ from English, a missing key or plural category in an
 *             ENABLED locale, code that asks for a key English lacks.
 *   warnings: unused keys, keys without a meta.json description.
 */
import { readdirSync, readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { catalogBundles, layoutProblems, listBundles, listSources, problems, readJson, readLocale } from './i18n-lib.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..');
const dir = join(root, 'messages');

const locales = readFileSync(join(root, 'src/i18n/locales.ts'), 'utf8');
const enabled = [...(/export const ENABLED_LOCALES[^=]*=\s*\[([^\]]*)\]/.exec(locales)?.[1] ?? '').matchAll(/'([^']+)'/g)].map((m) => m[1]);
const known = [...(/export const LOCALES = \[([^\]]*)\]/.exec(locales)?.[1] ?? '').matchAll(/'([^']+)'/g)].map((m) => m[1]);

const catalogs = {};
const bundles = {};
const errors = [];
for (const f of readdirSync(dir, { withFileTypes: true })) {
  if (!f.isDirectory()) {
    if (f.name !== 'meta.json') errors.push(`messages/${f.name}: catalogs live in messages/{locale}/{bundle}.json`);
    continue;
  }
  if (!known.includes(f.name)) errors.push(`messages/${f.name}/: "${f.name}" is not in LOCALES (src/i18n/locales.ts)`);
}
// A locale without a directory has no translations yet: an empty catalog (it falls back to English).
for (const l of known) {
  catalogs[l] = readLocale(join(dir, l));
  bundles[l] = listBundles(join(dir, l));
}
errors.push(...layoutProblems({ bundles, catalog: catalogBundles(readFileSync(join(root, 'src/i18n/catalog.ts'), 'utf8')) }));

const sources = listSources(join(root, 'src')).map((p) => readFileSync(p, 'utf8'));
const res = problems({ catalogs, enabled, meta: readJson(join(dir, 'meta.json')), sources });
errors.push(...res.errors);
for (const w of res.warnings) console.warn(`warn  ${w}`);
for (const e of errors) console.error(`error ${e}`);
console.log(`i18n:check ${Object.keys(catalogs).length} catalogs, enabled: ${enabled.join(', ')}, ${errors.length} errors, ${res.warnings.length} warnings`);
process.exit(errors.length ? 1 : 0);
