#!/usr/bin/env node
/**
 * End-to-end smoke test of the built site, run in a real browser against dist/ served below /SMSI/
 * (exactly how GitHub Pages serves a project site). SITE_BASE overrides the base path.
 *
 *   npm run build && npm run smoke
 *
 * Checks the landing page, the docs, and the browser-only demo: seeded dashboard, compose -> schedule,
 * the calendar, an MCP connection on the Developer page and the banner's Reset. Fails on any console
 * error, page error or failed local request. Screenshots go to SMOKE_OUT (default: a temp folder).
 */
import { mkdirSync, mkdtempSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { launch } from './lib.mjs';
import { startServer } from './serve.mjs';

const here = dirname(fileURLToPath(import.meta.url));
const dist = resolve(here, process.env.SITE_DIST ?? '../dist');
const PORT = Number(process.env.SMOKE_PORT ?? 4190);
const ORIGIN = `http://127.0.0.1:${PORT}`;
// The base the site was built for (SITE_BASE, default /SMSI/ as on GitHub Pages project sites).
const BASE = `/${(process.env.SITE_BASE ?? '/SMSI/').replace(/^\/+|\/+$/g, '')}/`.replace(/^\/\/$/, '/');
const SITE = `${ORIGIN}${BASE.slice(0, -1)}`;
const escRe = (x) => x.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
const out = process.env.SMOKE_OUT ? resolve(process.env.SMOKE_OUT) : mkdtempSync(join(tmpdir(), 'site-smoke-'));
mkdirSync(out, { recursive: true });

const server = await startServer({ dir: dist, port: PORT, base: BASE });
const browser = await launch();
const ctx = await browser.newContext({ viewport: { width: 1280, height: 800 }, locale: 'en-GB', timezoneId: 'UTC' });
const page = await ctx.newPage();

// Only the local server is reachable in the test; the Mermaid CDN is an optional enhancement (the docs fall back to its source).
await page.route((u) => u.hostname !== '127.0.0.1', (route) => route.abort());
const problems = [];
const external = (url) => !url.startsWith(ORIGIN);
page.on('pageerror', (e) => problems.push(`pageerror: ${e.message}`));
page.on('console', (m) => {
  if (m.type() !== 'error' && m.type() !== 'warning') return;
  if (external(m.location().url ?? '')) return;
  problems.push(`console.${m.type()}: ${m.text()}`);
});
page.on('response', (r) => {
  if (r.status() >= 400 && !external(r.url())) problems.push(`HTTP ${r.status()}: ${r.url()}`);
});

let failures = 0;
async function step(name, fn) {
  const before = problems.length;
  try {
    await fn();
    const fresh = problems.slice(before);
    if (fresh.length) throw new Error(`browser problems:\n    ${fresh.join('\n    ')}`);
    console.log(`  ok   ${name}`);
  } catch (e) {
    failures += 1;
    console.log(`  FAIL ${name}\n    ${String(e.message ?? e).split('\n').join('\n    ')}`);
    await page.screenshot({ path: join(out, `fail-${failures}.png`) }).catch(() => {});
  }
}
const expect = (cond, msg) => {
  if (!cond) throw new Error(msg);
};
const shot = (name) => page.screenshot({ path: join(out, `${name}.png`) });
const visible = (locator, timeout = 10_000) => locator.first().waitFor({ state: 'visible', timeout });

// A day that is always in the future, and an hour that sorts first in the calendar cell.
const tomorrow = new Date(Date.now() + 86_400_000);
const dateStr = tomorrow.toISOString().slice(0, 10);
const sameMonth = tomorrow.getUTCMonth() === new Date().getUTCMonth();
const TITLE = `Smoke test post ${Date.now().toString(36)}`;
const monthGoto = async () => {
  if (!sameMonth) await page.getByRole('button', { name: 'Next' }).click();
};

console.log(`site smoke: ${dist} at ${SITE}/`);

await step('landing page', async () => {
  await page.goto(`${SITE}/`);
  await visible(page.getByRole('heading', { level: 1, name: /One place to publish/ }));
  const demo = page.getByRole('link', { name: 'Try the demo' }).first();
  expect((await demo.getAttribute('href')) === `${BASE}demo/`, `Try the demo should link to ${BASE}demo/`);
  const docs = page.getByRole('link', { name: /docs/i }).first();
  expect((await docs.getAttribute('href'))?.startsWith(`${BASE}docs`), `Docs link should point into ${BASE}docs/`);
  const rows = await page.locator('table.networks tbody tr').allInnerTexts();
  expect(/LinkedIn[\s\S]*Live/.test(rows[0]) && /Telegram[\s\S]*Live/.test(rows[1]), 'LinkedIn and Telegram should be Live');
  expect(rows.some((r) => /Not available yet/.test(r)), 'other networks should say "Not available yet"');
  await visible(page.getByRole('heading', { name: /tools, each tied to one scope/ }));
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
  expect(overflow <= 0, `horizontal overflow of ${overflow}px`);
  await shot('01-landing');
});

await step('docs pages render', async () => {
  for (const [path, heading] of [
    ['docs/', 'Overview'],
    ['docs/getting-started/', 'Getting started'],
    ['docs/api/', 'REST API'],
    ['docs/mcp/', 'MCP server'],
    ['docs/architecture/', 'Architecture and contract'],
  ]) {
    await page.goto(`${SITE}/${path}`);
    await visible(page.getByRole('heading', { level: 1, name: heading }));
  }
  // Without the CDN the diagram stays readable as source.
  expect((await page.locator('pre.mermaid').count()) >= 1, 'the architecture page should contain a diagram block');
  await shot('02-docs-architecture');
});

await step('unknown page shows the 404', async () => {
  const mark = problems.length;
  const res = await page.goto(`${SITE}/nope/`);
  expect(res?.status() === 404, `expected 404, got ${res?.status()}`);
  await visible(page.getByRole('heading', { name: /does not exist/i }));
  // A 404 page is expected to log its own 404: forget what this step recorded.
  problems.length = mark;
});

await step('demo opens signed in with seeded data', async () => {
  await page.goto(`${SITE}/`);
  await page.getByRole('link', { name: 'Try the demo' }).first().click();
  await page.waitForURL(new RegExp(`${escRe(BASE)}demo/dashboard/?$`), { timeout: 15_000 });
  await visible(page.getByText('data stays in your browser'));
  await visible(page.getByRole('heading', { name: 'Upcoming' }));
  await visible(page.getByText('Release 2.5 teaser'));
  await visible(page.getByText('Connected accounts'));
  await shot('03-demo-dashboard');
});

await step('compose and schedule a post', async () => {
  await page.goto(`${SITE}/demo/compose/`);
  await visible(page.getByRole('group', { name: 'Publish to' }));
  await page.getByLabel('Title (optional)').fill(TITLE);
  await page.getByRole('button', { name: /Jordan Lee/ }).click();
  await page.getByRole('button', { name: /Studio Updates/ }).click();
  await page.getByLabel('Post content').fill('A smoke-test post, written in the browser demo and scheduled for tomorrow.');
  await page.locator('#sched-date').fill(dateStr);
  await page.locator('#sched-time').fill('00:05');
  await page.locator('#sched-time').blur();
  await page.getByRole('button', { name: 'Schedule', exact: true }).click();
  await page.waitForURL(/\/demo\/posts\//, { timeout: 15_000 });
  await visible(page.getByRole('heading', { name: TITLE }));
  await visible(page.getByText('Scheduled', { exact: true }));
  await shot('04-demo-post-scheduled');
});

await step('calendar shows the scheduled post', async () => {
  await page.goto(`${SITE}/demo/calendar/`);
  await visible(page.getByRole('heading', { name: /\d{4}/ }));
  await monthGoto();
  await visible(page.getByRole('link', { name: new RegExp(TITLE) }));
  await shot('05-demo-calendar');
  await page.getByRole('link', { name: new RegExp(TITLE) }).first().click();
  await page.waitForURL(/\/demo\/posts\//);
  await visible(page.getByRole('heading', { name: TITLE }));
});

await step('developer page creates an MCP connection', async () => {
  await page.goto(`${SITE}/demo/developer/mcp/`);
  await visible(page.getByRole('heading', { name: 'Connect an AI agent' }));
  await page.getByLabel('Connection name').fill('Smoke agent');
  await page.getByRole('button', { name: 'Create connection' }).click();
  const created = page.getByTestId('mcp-created');
  await visible(created);
  expect(/sk_live_/.test(await created.innerText()), 'the one-time key should be shown');
  await shot('06-demo-mcp');
});

await step('state survives a reload, Reset restores the seed', async () => {
  await page.goto(`${SITE}/demo/calendar/`);
  await visible(page.getByRole('heading', { name: /\d{4}/ }));
  await monthGoto();
  await visible(page.getByRole('link', { name: new RegExp(TITLE) }));
  await page.getByRole('button', { name: 'Reset' }).click();
  await page.waitForURL(/\/demo\/dashboard\/?$/, { timeout: 15_000 });
  await visible(page.getByText('Release 2.5 teaser'));
  await page.goto(`${SITE}/demo/calendar/`);
  await visible(page.getByRole('heading', { name: /\d{4}/ }));
  await monthGoto();
  await page.waitForTimeout(500);
  expect((await page.getByRole('link', { name: new RegExp(TITLE) }).count()) === 0, 'the smoke post should be gone after Reset');
});

await step('demo works on a phone', async () => {
  const phone = await browser.newContext({ viewport: { width: 390, height: 844 }, locale: 'en-GB', timezoneId: 'UTC', isMobile: true, hasTouch: true });
  const p = await phone.newPage();
  const errs = [];
  p.on('pageerror', (e) => errs.push(e.message));
  p.on('console', (m) => m.type() === 'error' && !external(m.location().url ?? '') && errs.push(m.text()));
  await p.goto(`${SITE}/demo/dashboard/`);
  await p.getByRole('heading', { name: 'Upcoming' }).first().waitFor({ state: 'visible' });
  await p.screenshot({ path: join(out, '07-demo-mobile.png') });
  await phone.close();
  expect(errs.length === 0, `phone console problems: ${errs.join('; ')}`);
});

await browser.close();
server.close();
console.log(failures === 0 ? `site smoke passed (screenshots in ${out})` : `site smoke FAILED: ${failures} step(s) (screenshots in ${out})`);
process.exit(failures === 0 ? 0 : 1);
