/** Pure parts of `npm run budget`: which JS files a route loads first, and whether that fits the budget. */
import { gzipSync } from 'node:zlib';

const isScript = (file) => file.endsWith('.js');

/** Page keys of app-build-manifest.json for a route: `/login` -> `/(auth)/login/page`; `/` -> `/page`. */
export function pageKeyFor(route, pages) {
  const suffix = route === '/' ? '/page' : `${route}/page`;
  const matches = Object.keys(pages).filter((key) => key === suffix || key.replace(/^\/\([^/)]+\)/, '') === suffix);
  if (matches.length !== 1) {
    throw new Error(`route ${route}: expected exactly one page entry in app-build-manifest.json, found ${matches.length}`);
  }
  return matches[0];
}

/** Files Next adds per segment level beside `layout` (the root level is `/not-found`, `/error`, ...). */
const SEGMENT_FILES = ['layout', 'template', 'error', 'loading', 'not-found'];

/**
 * The scripts a first visit to `route` downloads: the root main files and, for the root and every segment above the page
 * (route group, nested), its layout, template, error, loading and not-found entries when present, then the page entry.
 * The root `not-found` is a client component every page ships. Polyfills are left out on purpose: `nomodule`, never
 * loaded by the browsers we target, and Next's own table omits them too. Next's printed table also omits layout chunks;
 * this does not.
 */
export function routeScripts(route, { appManifest, buildManifest }) {
  const pages = appManifest.pages;
  const pageKey = pageKeyFor(route, pages);
  const segments = pageKey.split('/').slice(1, -1);
  const prefixes = [''];
  for (let depth = 1; depth <= segments.length; depth += 1) prefixes.push(`/${segments.slice(0, depth).join('/')}`);
  const keys = prefixes.flatMap((prefix) => SEGMENT_FILES.map((name) => `${prefix}/${name}`)).filter((key) => key in pages);
  const files = new Set(buildManifest.rootMainFiles);
  for (const key of [...keys, pageKey]) {
    for (const file of pages[key]) files.add(file);
  }
  return [...files].filter(isScript).sort();
}

export const gzipBytes = (buffer) => gzipSync(buffer, { level: 6 }).length;

/** Budgets are in kB of 1000 bytes, the unit Next prints. */
export const toKb = (bytes) => Math.round(bytes / 10) / 100;

/** One row per budgeted route; `over` is true when the measured size is above the budget. */
export function compareBudgets(measured, budgets) {
  return Object.entries(budgets).map(([route, budgetKb]) => {
    const bytes = measured[route];
    if (bytes === undefined) throw new Error(`route ${route} has a budget but was not measured`);
    return { route, kb: toKb(bytes), budgetKb, over: bytes > budgetKb * 1000 };
  });
}

export function formatTable(rows) {
  const width = Math.max(5, ...rows.map((r) => r.route.length));
  const line = (a, b, c, d) => `${a.padEnd(width)}  ${b.padStart(10)}  ${c.padStart(10)}  ${d}`;
  return [
    line('Route', 'gzip kB', 'budget kB', ''),
    ...rows.map((r) => line(r.route, r.kb.toFixed(1), r.budgetKb.toFixed(1), r.over ? `OVER by ${(r.kb - r.budgetKb).toFixed(1)} kB` : 'ok')),
  ].join('\n');
}

/** Files in `contents` (path -> text) that contain `needle`; used for the regression guards. */
export const filesContaining = (contents, needle) => Object.keys(contents).filter((file) => contents[file].includes(needle));

/**
 * A marker guard. `forbidden` and `expected` map file -> text. Fails when the marker is in a forbidden file, and also
 * when it is in no expected file: then the marker was renamed or dropped and the guard would pass without looking.
 */
export function markerProblems({ marker, label, forbidden, expected, fix }) {
  const problems = filesContaining(forbidden, marker).map((file) => `guard: ${label} ("${marker}") is in ${file}; ${fix}`);
  if (filesContaining(expected, marker).length === 0) {
    problems.push(`guard: "${marker}" is in no (app) chunk any more, so the ${label} check cannot detect anything. Update the marker in scripts/bundle-budget.mjs.`);
  }
  return problems;
}
