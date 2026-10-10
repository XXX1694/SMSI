/**
 * Browser-side check that no screen shows a raw message key (`posts.untitled`) in place of text: the symptom of a route
 * whose scope lacks a namespace, or of text shown before a locale's bundles arrived. Used by smoke.mjs (against the mock
 * API) and by the demo browser check. Needs a Playwright `page`.
 */
import { readdirSync, readFileSync } from 'node:fs';

const MESSAGES = new URL('../messages/en/', import.meta.url);
const NAMESPACES = readdirSync(MESSAGES).map((file) => file.replace('.json', ''));

function flatten(node, prefix, out) {
  for (const [key, value] of Object.entries(node)) {
    if (typeof value === 'string') out.push(prefix + key);
    else flatten(value, `${prefix}${key}.`, out);
  }
  return out;
}
const KEYS = NAMESPACES.flatMap((ns) => flatten(JSON.parse(readFileSync(new URL(`${ns}.json`, MESSAGES), 'utf8')), `${ns}.`, []));

/** Signed-in routes (the post detail is reached from the list, so the id comes from the data). */
export const APP_ROUTES = ['/dashboard', '/compose', '/posts', '/calendar', '/media', '/analytics', '/accounts', '/approvals', '/developer', '/developer/mcp', '/settings'];
export const AUTH_ROUTES = ['/login', '/register', '/verify-email', '/forgot-password', '/reset-password', '/signup/complete'];
/** An unknown URL renders app/not-found.tsx (root scope only). */
export const NOT_FOUND_ROUTES = ['/this-page-does-not-exist'];
export const LOCALES = ['en', 'ru'];

/** Tokens of the page's visible text and text attributes that are catalog keys, or look like `namespace.camelCase.path`. */
export async function rawKeysOnPage(page) {
  return page.evaluate(
    ({ namespaces, keys }) => {
      const known = new Set(keys);
      const looksLikeKey = (token) => {
        const parts = token.split('.');
        if (!namespaces.includes(parts[0]) || parts.length < 2) return false;
        return known.has(token) || parts.length > 2 || /[A-Z]/.test(token);
      };
      const texts = [document.body.innerText];
      for (const el of document.querySelectorAll('[aria-label],[title],[placeholder],[alt]')) {
        for (const attr of ['aria-label', 'title', 'placeholder', 'alt']) texts.push(el.getAttribute(attr) ?? '');
      }
      const found = new Set();
      for (const text of texts) {
        for (const token of text.split(/[\s,;:()[\]"'“”«»]+/)) {
          const clean = token.replace(/\.+$/, '');
          if (clean && looksLikeKey(clean)) found.add(clean);
        }
      }
      return [...found];
    },
    { namespaces: NAMESPACES, keys: KEYS },
  );
}

async function settle(page, locale) {
  await page.waitForLoadState('networkidle');
  await page.waitForFunction((l) => document.documentElement.lang === l && !document.documentElement.hasAttribute('data-i18n-pending'), locale, { timeout: 8000 });
  await page.waitForTimeout(400); // data views fetch after mount; let the first paint of their text land
}

/**
 * Visits each route in each locale and returns `[{ locale, route, keys }]` for every screen that shows a raw key.
 * `open(route)` navigates (it differs between the server build and the static demo).
 */
export async function scanForRawKeys(page, routes, { locales = LOCALES, open }) {
  const findings = [];
  for (const locale of locales) {
    await page.evaluate((l) => localStorage.setItem('steerpost_locale', l), locale);
    for (const route of routes) {
      await open(route);
      await settle(page, locale);
      const keys = await rawKeysOnPage(page);
      if (keys.length) findings.push({ locale, route, keys });
    }
  }
  return findings;
}
