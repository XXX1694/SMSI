#!/usr/bin/env node
/**
 * `npm run i18n:check` (part of `npm run lint`). Offline, a few seconds.
 *   errors:   a catalog key that English lacks, invalid ICU, placeholders that differ from English, a missing key or
 *             plural category in an ENABLED locale, code that asks for a key English lacks.
 *   warnings: unused keys, keys without a meta.json description.
 */
import { readdirSync, readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { listSources, problems, readJson } from './i18n-lib.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..');
const dir = join(root, 'messages');

const locales = readFileSync(join(root, 'src/i18n/locales.ts'), 'utf8');
const enabled = [...(/export const ENABLED_LOCALES[^=]*=\s*\[([^\]]*)\]/.exec(locales)?.[1] ?? '').matchAll(/'([^']+)'/g)].map((m) => m[1]);
const known = [...(/export const LOCALES = \[([^\]]*)\]/.exec(locales)?.[1] ?? '').matchAll(/'([^']+)'/g)].map((m) => m[1]);

const catalogs = {};
const errors = [];
for (const f of readdirSync(dir)) {
  if (!f.endsWith('.json') || f === 'meta.json') continue;
  const locale = f.replace(/\.json$/, '');
  if (!known.includes(locale)) errors.push(`messages/${f}: "${locale}" is not in LOCALES (src/i18n/locales.ts)`);
  catalogs[locale] = readJson(join(dir, f));
}
for (const l of known) if (!(l in catalogs)) errors.push(`messages/${l}.json is missing`);

const sources = listSources(join(root, 'src')).map((p) => readFileSync(p, 'utf8'));
const res = problems({ catalogs, enabled, meta: readJson(join(dir, 'meta.json')), sources });
errors.push(...res.errors);
for (const w of res.warnings) console.warn(`warn  ${w}`);
for (const e of errors) console.error(`error ${e}`);
console.log(`i18n:check ${Object.keys(catalogs).length} catalogs, enabled: ${enabled.join(', ')}, ${errors.length} errors, ${res.warnings.length} warnings`);
process.exit(errors.length ? 1 : 0);
