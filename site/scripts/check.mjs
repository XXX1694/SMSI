#!/usr/bin/env node
/**
 * Link check for the built site (no browser needed): every internal href/src in dist/**.html must
 * resolve to a file, and every #fragment on the site's own pages must exist on its target page.
 *
 *   node scripts/check.mjs [dist=dist] [base=/steerpost/]
 */
import { existsSync, readFileSync, readdirSync, statSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { LOCALES } from '../i18n/locales.mjs';

const here = dirname(fileURLToPath(import.meta.url));
const dist = resolve(here, '..', process.argv[2] ?? 'dist');
const base = process.argv[3] ?? '/steerpost/';
const problems = [];

const walk = (dir) =>
  readdirSync(dir, { withFileTypes: true }).flatMap((e) => (e.isDirectory() ? walk(join(dir, e.name)) : [join(dir, e.name)]));
const htmlFiles = walk(dist).filter((f) => f.endsWith('.html'));
const cache = new Map();
const idsOf = (file) => {
  if (!cache.has(file)) cache.set(file, new Set([...readFileSync(file, 'utf8').matchAll(/\sid="([^"]+)"/g)].map((m) => m[1])));
  return cache.get(file);
};

function target(urlPath) {
  const rel = decodeURIComponent(urlPath.slice(base.length));
  const p = join(dist, rel);
  if (existsSync(p) && statSync(p).isFile()) return p;
  if (existsSync(join(p, 'index.html'))) return join(p, 'index.html');
  return null;
}

let checked = 0;
for (const file of htmlFiles) {
  const page = file.slice(dist.length + 1);
  const inDemo = page.startsWith('demo/');
  const html = readFileSync(file, 'utf8');
  // The demo's own pages are Next output: check that their assets exist, not their in-page anchors.
  for (const m of html.matchAll(/\s(?:href|src|srcset)="([^"]+)"/g)) {
    for (const raw of m[1].split(',').map((s) => s.trim().split(/\s+/)[0])) {
      if (!raw || /^(?:[a-z][a-z0-9+.-]*:|\/\/|data:)/i.test(raw) && !raw.startsWith(base)) continue;
      let url = raw;
      if (!url.startsWith('#') && !url.startsWith('/')) continue; // relative refs are not used by this site
      let frag = '';
      if (url.includes('#')) [url, frag] = [url.slice(0, url.indexOf('#')), url.slice(url.indexOf('#') + 1)];
      url = url.split('?')[0];
      checked += 1;
      if (url === '') {
        if (!inDemo && frag && !idsOf(file).has(frag)) problems.push(`${page}: #${frag} not found on this page`);
        continue;
      }
      if (!url.startsWith(base)) {
        problems.push(`${page}: link outside the site base: ${raw}`);
        continue;
      }
      const t = target(url);
      if (!t) problems.push(`${page}: broken link ${raw}`);
      else if (!inDemo && frag && t.endsWith('.html') && !idsOf(t).has(frag)) problems.push(`${page}: ${raw} (no #${frag} in ${t.slice(dist.length + 1)})`);
    }
  }
}

for (const must of ['index.html', 'docs/index.html', 'docs/mcp/index.html', 'docs/architecture/index.html', 'demo/index.html', 'demo/dashboard/index.html', '404.html']) {
  if (!existsSync(join(dist, must))) problems.push(`missing ${must}`);
}

// ---- landing locales (D-021): a page, a complete catalog and complete hreflang for every locale
{
  const i18nDir = resolve(here, '../i18n');
  const read = (f) => readFileSync(f, 'utf8');
  const en = JSON.parse(read(join(i18nDir, 'landing.en.json')));
  const listed = LOCALES.filter((l) => !l.hidden);
  const slugPath = (l) => `${base}${l.slug ? `${l.slug}/` : ''}`;
  for (const l of LOCALES) {
    const catFile = join(i18nDir, `landing.${l.code}.json`);
    if (!existsSync(catFile)) {
      problems.push(`i18n: no catalog site/i18n/landing.${l.code}.json`);
      continue;
    }
    const cat = JSON.parse(read(catFile));
    const missing = Object.keys(en).filter((k) => !(k in cat));
    const extra = Object.keys(cat).filter((k) => !(k in en));
    if (missing.length) problems.push(`i18n ${l.code}: missing keys: ${missing.join(', ')}`);
    if (extra.length) problems.push(`i18n ${l.code}: keys English does not have: ${extra.join(', ')}`);
    if (l.code !== 'en') {
      const empty = Object.entries(cat).filter(([, v]) => (Array.isArray(v) ? v.length === 0 : String(v).trim() === '')).map(([k]) => k);
      if (empty.length) problems.push(`i18n ${l.code}: empty values: ${empty.join(', ')}`);
      const same = Object.keys(en).filter((k) => !/^(meta\.ogAlt|route\.agent|facts\.)/.test(k) && typeof en[k] === 'string' && en[k].length > 25 && cat[k] === en[k]);
      if (same.length) problems.push(`i18n ${l.code}: untranslated (same as English): ${same.join(', ')}`);
      for (const k of Object.keys(en)) {
        if (Array.isArray(en[k]) !== Array.isArray(cat[k])) problems.push(`i18n ${l.code}: "${k}" must be ${Array.isArray(en[k]) ? 'an array' : 'a string'}`);
      }
      if (!/Beta|Бета|beta|ベータ|测试版|بيتا|تجريبية|bêta|Beta-/.test(cat['footer.beta'] ?? '')) problems.push(`i18n ${l.code}: footer.beta must say "Beta translation" in the language`);
    }
    const file = join(dist, l.slug, 'index.html');
    if (!existsSync(file)) {
      problems.push(`i18n ${l.code}: page ${l.slug || '/'} is not built`);
      continue;
    }
    const html = read(file);
    if (/\{\{[\w:.]+\}\}|\{[a-zA-Z]+Count[,}]/.test(html)) problems.push(`i18n ${l.code}: a template token or ICU placeholder is left in the page`);
    if (!html.includes(`<html lang="${l.lang}" dir="${l.dir}">`)) problems.push(`i18n ${l.code}: <html> must be lang="${l.lang}" dir="${l.dir}"`);
    if (!html.includes(`<link rel="canonical" href="`) || !html.match(/rel="canonical" href="([^"]+)"/)?.[1]?.endsWith(slugPath(l))) problems.push(`i18n ${l.code}: canonical must point to ${slugPath(l)}`);
    const alternates = [...html.matchAll(/<link rel="alternate" hreflang="([^"]+)" href="([^"]+)">/g)].map((m) => [m[1], m[2]]);
    if (l.hidden) {
      if (!/<meta name="robots" content="noindex/.test(html)) problems.push(`i18n ${l.code}: hidden locale must be noindex`);
      if (alternates.length) problems.push(`i18n ${l.code}: hidden locale must not declare hreflang alternates`);
    } else {
      for (const o of [...listed.map((x) => [x.hreflang, slugPath(x)]), ['x-default', base]]) {
        if (!alternates.some(([h, u]) => h === o[0] && u.endsWith(o[1]))) problems.push(`i18n ${l.code}: hreflang ${o[0]} -> ${o[1]} is missing`);
      }
      if (alternates.length !== listed.length + 1) problems.push(`i18n ${l.code}: expected ${listed.length + 1} hreflang links, found ${alternates.length}`);
      if (/noindex/.test(html)) problems.push(`i18n ${l.code}: a listed locale must be indexable`);
    }
  }
  // A hidden locale is not linked from any page, and the sitemap lists exactly the listed ones.
  for (const l of LOCALES.filter((x) => x.hidden)) {
    for (const f of htmlFiles) {
      const page = f.slice(dist.length + 1);
      if (page.startsWith(`${l.slug}/`) || page.startsWith('demo/')) continue;
      if (readFileSync(f, 'utf8').includes(`href="${slugPath(l)}"`)) problems.push(`i18n: hidden locale ${l.code} is linked from ${page}`);
    }
  }
  const sitemap = existsSync(join(dist, 'sitemap.xml')) ? read(join(dist, 'sitemap.xml')) : '';
  for (const l of LOCALES) {
    const inMap = sitemap.includes(`${slugPath(l)}</loc>`);
    if (!l.hidden && !inMap) problems.push(`i18n: sitemap.xml lacks ${slugPath(l)}`);
    if (l.hidden && inMap) problems.push(`i18n: sitemap.xml lists the hidden locale ${l.code}`);
  }
  if (!existsSync(join(dist, 'en/index.html'))) problems.push('i18n: /en/ redirect page is missing');
  else if (!read(join(dist, 'en/index.html')).includes(`url=${base}"`)) problems.push('i18n: /en/ must forward to the English root');
}

if (problems.length) {
  console.error(`site check: ${problems.length} problem(s)\n${[...new Set(problems)].map((p) => `  - ${p}`).join('\n')}`);
  process.exit(1);
}
console.log(`site check: ${htmlFiles.length} pages, ${checked} internal links OK`);
