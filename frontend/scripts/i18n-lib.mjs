/**
 * Shared by `scripts/i18n-check.mjs` (CI) and `tests/i18n-catalogs.test.ts`. Pure functions, no side effects at import.
 * Rules come from docs/copy/translation-process.md section 3; the rest of them land as warnings later.
 */
import { parse, TYPE } from '@formatjs/icu-messageformat-parser';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';

/** `{ a: { b: 'x' } }` -> `{ 'a.b': 'x' }`. Non-string leaves are reported by `problems`. */
export function flatten(obj, prefix = '') {
  const out = {};
  for (const [k, v] of Object.entries(obj)) {
    const key = prefix ? `${prefix}.${k}` : k;
    if (v !== null && typeof v === 'object' && !Array.isArray(v)) Object.assign(out, flatten(v, key));
    else out[key] = v;
  }
  return out;
}

/** Argument names, plural/select types and rich tags used by a message, or throws on a syntax error. */
export function shape(message) {
  const args = new Set();
  const tags = new Set();
  const walk = (els) => {
    for (const el of els) {
      switch (el.type) {
        case TYPE.argument:
        case TYPE.number:
        case TYPE.date:
        case TYPE.time:
          args.add(`${el.value}:${el.type}`);
          break;
        case TYPE.plural:
        case TYPE.select:
          args.add(`${el.value}:${el.type}${el.pluralType === 'ordinal' ? 'o' : ''}`);
          for (const o of Object.values(el.options)) walk(o.value);
          break;
        case TYPE.tag:
          tags.add(el.value);
          walk(el.children);
          break;
        default:
      }
    }
  };
  walk(parse(message, { requiresOtherClause: true }));
  return { args, tags };
}

function pluralCategoriesMissing(message, locale) {
  const missing = [];
  const wanted = new Intl.PluralRules(locale).resolvedOptions().pluralCategories;
  const walk = (els) => {
    for (const el of els) {
      if (el.type === TYPE.plural) {
        const cats = new Intl.PluralRules(locale, { type: el.pluralType === 'ordinal' ? 'ordinal' : 'cardinal' }).resolvedOptions().pluralCategories;
        for (const c of el.pluralType === 'ordinal' ? cats : wanted) if (!(c in el.options)) missing.push(`${el.value}:${c}`);
        for (const o of Object.values(el.options)) walk(o.value);
      } else if (el.type === TYPE.select) for (const o of Object.values(el.options)) walk(o.value);
      else if (el.type === TYPE.tag) walk(el.children);
    }
  };
  walk(parse(message));
  return missing;
}

const SRC_EXT = /\.(ts|tsx)$/;
export function listSources(dir) {
  const out = [];
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    if (statSync(p).isDirectory()) out.push(...listSources(p));
    else if (SRC_EXT.test(name) && !name.endsWith('.d.ts')) out.push(p);
  }
  return out;
}

/**
 * Keys that source code asks for with a literal: `const t = useTranslations('nav'); t('dashboard')` -> `nav.dashboard`.
 * A call is resolved against the nearest earlier `useTranslations` in the same file. Dynamic keys are not seen here.
 */
export function usedKeys(source) {
  const defs = [...source.matchAll(/\b(?:const|let)\s+(\w+)\s*=\s*useTranslations\(\s*(?:['"]([\w.]+)['"])?\s*\)/g)].map((m) => ({
    at: m.index,
    name: m[1],
    ns: m[2] ?? '',
  }));
  const keys = [];
  for (const d of defs) {
    const re = new RegExp(`\\b${d.name}(?:\\.rich)?\\(\\s*['"]([\\w.]+)['"]`, 'g');
    for (const m of source.matchAll(re)) {
      const owner = defs.filter((x) => x.name === d.name && x.at <= m.index).pop();
      if (owner === d) keys.push(d.ns ? `${d.ns}.${m[1]}` : m[1]);
    }
  }
  return keys;
}

/** String literals in a source file (cheap, for the "unused key" heuristic). */
export function literals(source) {
  return new Set([...source.matchAll(/['"`]([\w.]+)['"`]/g)].map((m) => m[1]));
}

/**
 * @param {{ catalogs: Record<string, object>, enabled: string[], meta?: object, sources?: string[] }} input
 * `catalogs` maps a locale to its parsed JSON; `en` is required.
 * @returns {{ errors: string[], warnings: string[] }}
 */
export function problems({ catalogs, enabled, meta = {}, sources = [] }) {
  const errors = [];
  const warnings = [];
  const en = catalogs.en;
  if (!en) return { errors: ['messages/en.json is missing'], warnings };
  const enFlat = flatten(en);
  for (const [k, v] of Object.entries(enFlat)) {
    if (typeof v !== 'string') errors.push(`en: ${k} must be a string`);
    else
      try {
        shape(v);
      } catch (e) {
        errors.push(`en: ${k} is not valid ICU: ${e.message}`);
      }
  }
  for (const [locale, cat] of Object.entries(catalogs)) {
    if (locale === 'en') continue;
    const flat = flatten(cat);
    for (const [k, v] of Object.entries(flat)) {
      if (!(k in enFlat)) {
        errors.push(`${locale}: ${k} is not an English key`);
        continue;
      }
      if (typeof v !== 'string') {
        errors.push(`${locale}: ${k} must be a string`);
        continue;
      }
      try {
        const got = shape(v);
        const want = shape(enFlat[k]);
        const a = [...got.args].sort().join();
        const b = [...want.args].sort().join();
        if (a !== b) errors.push(`${locale}: ${k} placeholders differ from English (${a || 'none'} vs ${b || 'none'})`);
        if ([...got.tags].sort().join() !== [...want.tags].sort().join()) errors.push(`${locale}: ${k} rich tags differ from English`);
        if (enabled.includes(locale)) {
          const miss = pluralCategoriesMissing(v, locale);
          if (miss.length) errors.push(`${locale}: ${k} lacks plural categories ${miss.join(', ')}`);
        }
      } catch (e) {
        errors.push(`${locale}: ${k} is not valid ICU: ${e.message}`);
      }
    }
    if (enabled.includes(locale)) {
      for (const k of Object.keys(enFlat)) if (!(k in flat)) errors.push(`${locale}: missing key ${k} (locale is enabled)`);
    }
  }
  for (const k of Object.keys(meta)) if (!(k in enFlat)) errors.push(`meta.json: ${k} is not an English key`);
  for (const k of Object.keys(enFlat)) if (!(k in meta)) warnings.push(`meta.json: no description for ${k}`);

  const used = new Set();
  const lits = new Set();
  for (const src of sources) {
    for (const k of usedKeys(src)) used.add(k);
    for (const l of literals(src)) lits.add(l);
  }
  for (const k of used) if (!(k in enFlat)) errors.push(`code uses ${k}, which is not in messages/en.json`);
  for (const k of Object.keys(enFlat)) {
    if (used.has(k) || lits.has(k)) continue;
    const parts = k.split('.');
    // dynamic keys: t(item.labelKey) with useTranslations('nav') and the leaf written as a literal elsewhere
    const dynamic = parts.some((_, i) => i > 0 && lits.has(parts.slice(0, i).join('.')) && lits.has(parts.slice(i).join('.')));
    if (!dynamic) warnings.push(`unused key ${k}`);
  }
  return { errors, warnings };
}

export function readJson(path) {
  return JSON.parse(readFileSync(path, 'utf8'));
}
