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

if (problems.length) {
  console.error(`site check: ${problems.length} problem(s)\n${[...new Set(problems)].map((p) => `  - ${p}`).join('\n')}`);
  process.exit(1);
}
console.log(`site check: ${htmlFiles.length} pages, ${checked} internal links OK`);
