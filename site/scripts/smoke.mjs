#!/usr/bin/env node
/**
 * End-to-end smoke test of the built site, run in a real browser against dist/ served below /steerpost/
 * (exactly how GitHub Pages serves a project site). SITE_BASE overrides the base path.
 *
 *   npm run build && npm run smoke
 *
 * Checks the landing page (hero video, honest network list, scroll story, reduced motion, phone), the docs, and the browser-only demo: seeded dashboard, compose -> schedule,
 * the calendar, an MCP connection on the Developer page and the banner's Reset (with its confirmation). Fails on any console
 * error, page error or failed local request. Screenshots go to SMOKE_OUT (default: a temp folder).
 */
import { mkdirSync, mkdtempSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { existsSync } from 'node:fs';
import { LOCALES } from '../i18n/locales.mjs';
import { APP_ROUTES, AUTH_ROUTES, scanForRawKeys } from '../../frontend/scripts/i18n-raw-keys.mjs';
import { launch } from './lib.mjs';
import { startServer } from './serve.mjs';

const here = dirname(fileURLToPath(import.meta.url));
const dist = resolve(here, process.env.SITE_DIST ?? '../dist');
const PORT = Number(process.env.SMOKE_PORT ?? 4190);
const ORIGIN = `http://127.0.0.1:${PORT}`;
// The base the site was built for (SITE_BASE, default /steerpost/ as on GitHub Pages project sites).
const BASE = `/${(process.env.SITE_BASE ?? '/steerpost/').replace(/^\/+|\/+$/g, '')}/`.replace(/^\/\/$/, '/');
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
  await visible(page.getByRole('heading', { level: 1, name: /AI agents draft posts\.\s*You stay in control\./ }));
  const demo = page.getByRole('link', { name: 'Try the demo' }).first();
  expect((await demo.getAttribute('href')) === `${BASE}demo/`, `Try the demo should link to ${BASE}demo/`);
  const docs = page.getByRole('link', { name: /docs/i }).first();
  expect((await docs.getAttribute('href'))?.startsWith(`${BASE}docs`), `Docs link should point into ${BASE}docs/`);
  // Honest network list: exactly the five live networks, and the rest labelled as not available.
  const live = (await page.locator('.net-live .net-rows li strong').allInnerTexts()).join(',');
  expect(live === 'LinkedIn,Telegram,Discord,Mastodon,Bluesky', `live networks should be the five documented ones, got ${live}`);
  expect((await page.locator('.net-off').innerText()).includes('Not available yet'), 'other networks should say "Not available yet"');
  expect((await page.locator('.marquee li:not([aria-hidden]) span').allInnerTexts()).length === 5, 'the marquee names only the five live networks');
  await visible(page.locator('details.tools summary'));
  expect(/All \d+ tools/.test(await page.locator('details.tools summary').innerText()), 'the tools list should be reachable');
  await page.waitForSelector('.stage.is-playing', { timeout: 10_000 });
  await page.waitForTimeout(600);
  expect(await page.evaluate(() => document.querySelector('.stage video').currentTime > 0), 'the hero video should be playing');
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
  expect(overflow <= 0, `horizontal overflow of ${overflow}px`);
  await shot('01-landing');
});

await step('landing scroll motion and story', async () => {
  await page.goto(`${SITE}/`);
  await page.locator('.facts-band').scrollIntoViewIfNeeded();
  await page.waitForTimeout(1500);
  expect((await page.locator('.num').first().innerText()).trim() === (await page.locator('.num').first().getAttribute('data-count')), 'counters should end on the true value');
  await page.locator('.step[data-step="2"]').scrollIntoViewIfNeeded();
  await page.waitForTimeout(600);
  expect((await page.locator('.story-stage').getAttribute('data-active')) === '2', 'the sticky picture should follow the active step');
  const revealed = await page.evaluate(() => [...document.querySelectorAll('.reveal')].filter((e) => getComputedStyle(e).opacity === '0' && e.getBoundingClientRect().top > 0 && e.getBoundingClientRect().top < innerHeight * 0.5).length);
  expect(revealed === 0, `${revealed} reveal element(s) in view are still hidden`);
});

await step('motion switch stops all motion, exposes its state and is remembered', async () => {
  await page.goto(`${SITE}/`);
  await page.waitForSelector('.stage.is-playing', { timeout: 10_000 });
  const btn = page.getByRole('button', { name: 'Pause motion' });
  expect((await btn.getAttribute('aria-pressed')) === 'false', 'Pause motion should start unpressed');
  await btn.focus();
  await page.keyboard.press('Enter');
  await page.waitForSelector('[data-motion][aria-pressed="true"]');
  await page.locator('.facts-band').scrollIntoViewIfNeeded();
  await page.waitForTimeout(300);
  const s = await page.evaluate(() => ({
    off: document.documentElement.classList.contains('motion-off'),
    video: document.querySelector('.stage video').paused,
    loops: ['.blob', '.marquee-track'].map((q) => getComputedStyle(document.querySelector(q)).animationPlayState),
    // scroll-driven animations (parallax, hero exit, clip reveals, the route) must not run either, and nothing may stay hidden
    running: document.getAnimations().filter((a) => a.playState === 'running' && !(a.timeline instanceof DocumentTimeline)).length,
    hiddenReveals: [...document.querySelectorAll('.reveal')].filter((e) => getComputedStyle(e).opacity === '0' && e.getBoundingClientRect().top < innerHeight).length,
    saved: localStorage.getItem('socialos_landing_motion'),
  }));
  expect(s.off && s.video && s.loops.every((x) => x === 'paused') && s.saved === 'off', `motion switch left something running: ${JSON.stringify(s)}`);
  expect(s.running === 0 && s.hiddenReveals === 0, `paused motion should stop scroll animations and show everything: ${JSON.stringify(s)}`);
  await page.reload();
  await page.waitForSelector('[data-motion][aria-pressed="true"]');
  expect(await page.evaluate(() => document.querySelector('.stage video').paused), 'the choice should survive a reload');
  await page.getByRole('button', { name: 'Pause motion' }).click();
  await page.waitForSelector('[data-motion][aria-pressed="false"]');
  await page.waitForSelector('.stage.is-playing', { timeout: 10_000 });
});

await step('a failing enhancement never hides the page', async () => {
  const ctx2 = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  const p = await ctx2.newPage();
  await p.route((u) => u.hostname !== '127.0.0.1' || u.pathname.endsWith('hero-flow.js'), (route) => route.abort());
  await p.addInitScript(() => { delete CanvasRenderingContext2D.prototype.roundRect; });
  await p.goto(`${SITE}/`);
  await p.waitForTimeout(800);
  await p.locator('.look').scrollIntoViewIfNeeded();
  await p.waitForTimeout(1200);
  const hidden = await p.evaluate(() => [...document.querySelectorAll('.reveal')].filter((e) => getComputedStyle(e).opacity === '0' && e.getBoundingClientRect().top > 0 && e.getBoundingClientRect().top < innerHeight * 0.5).length);
  expect(hidden === 0, `${hidden} section(s) stayed hidden after a script failure`);
  await ctx2.close();
});

await step('headings stay visible when the reveal script fails', async () => {
  const ctx3 = await browser.newContext({ viewport: { width: 1280, height: 800 }, javaScriptEnabled: false });
  const p = await ctx3.newPage();
  await p.goto(`${SITE}/`);
  const hidden = await p.evaluate(() => [...document.querySelectorAll('h1, h2')].filter((e) => getComputedStyle(e).opacity === '0' || e.getBoundingClientRect().width === 0).length);
  expect(hidden === 0, `${hidden} heading(s) hidden without JS`);
  await ctx3.close();
  const ctx4 = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  const q = await ctx4.newPage();
  await q.addInitScript(() => { window.IntersectionObserver = class { constructor() { throw new Error('no IO'); } }; });
  await q.goto(`${SITE}/`);
  await q.waitForTimeout(800);
  const clipped = await q.evaluate(() => [...document.querySelectorAll('.lp-h2')].filter((h) => [...h.querySelectorAll('.sw-i')].some((w) => getComputedStyle(w).transform !== 'none')).length);
  expect(clipped === 0, `${clipped} split heading(s) hidden after the reveal setup failed`);
  await ctx4.close();
});

await step('landing respects reduced motion and phones', async () => {
  const calm = await browser.newContext({ viewport: { width: 1280, height: 800 }, reducedMotion: 'reduce' });
  const p = await calm.newPage();
  await p.route((u) => u.hostname !== '127.0.0.1', (route) => route.abort());
  await p.goto(`${SITE}/`);
  await p.waitForTimeout(1200);
  const state = await p.evaluate(() => ({ src: document.querySelector('.stage video').getAttribute('src'), playing: !!document.querySelector('.stage.is-playing'), hidden: [...document.querySelectorAll('.reveal')].filter((e) => getComputedStyle(e).opacity === '0').length }));
  expect(!state.src && !state.playing, 'reduced motion must not load or play the video (the poster stays)');
  expect(await p.locator('[data-motion]').isHidden(), 'nothing moves under reduced motion, so no switch is offered');
  expect(state.hidden === 0, 'reduced motion must show every section without a reveal');
  await calm.close();
  const phone = await browser.newContext({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true });
  const q = await phone.newPage();
  await q.route((u) => u.hostname !== '127.0.0.1', (route) => route.abort());
  await q.goto(`${SITE}/`);
  await q.waitForTimeout(800);
  const m = await q.evaluate(() => ({ src: document.querySelector('.stage video').getAttribute('src'), over: document.documentElement.scrollWidth - innerWidth }));
  expect(!m.src, 'phones get the poster, not the video');
  expect(m.over <= 0, `horizontal overflow of ${m.over}px at 390px`);
  await phone.close();
});

await step('landing nav stays on one line and covers what scrolls under it', async () => {
  for (const [w, h] of [[320, 640], [360, 780], [390, 844], [414, 896], [768, 1024]]) {
    const ctx = await browser.newContext({ viewport: { width: w, height: h }, isMobile: w < 700, hasTouch: w < 700 });
    const p = await ctx.newPage();
    await p.route((u) => u.hostname !== '127.0.0.1', (route) => route.abort());
    await p.goto(`${SITE}/`);
    await p.waitForTimeout(600);
    const nav = await p.evaluate(() => [...document.querySelectorAll('.lp-nav .nav > a')].filter((a) => a.offsetParent).map((a) => ({ t: a.textContent.trim(), h: a.getBoundingClientRect().height, r: a.getBoundingClientRect().right })));
    expect(nav.every((a) => a.h < 50 && a.r <= w), `${w}px: nav links wrap or overflow: ${JSON.stringify(nav)}`);
    expect(w >= 768 || nav.every((a) => ['Docs', 'Try the demo'].includes(a.t)), `${w}px: only Docs and Try the demo should show: ${JSON.stringify(nav)}`);
    for (const y of [900, 1500, 2100, 2800, 3600]) {
      await p.evaluate((y) => scrollTo({ top: y, behavior: 'instant' }), y);
      await p.waitForTimeout(250);
      const bad = await p.evaluate(() => {
        const heads = [document.querySelector('.lp-nav-bar'), ...(getComputedStyle(document.querySelector('.story-stage')).position === 'sticky' && innerWidth <= 896 ? [document.querySelector('.story-stage')] : [])];
        const out = [];
        for (const el of document.querySelectorAll('h1, h2, h3, p, li, summary, figcaption')) {
          const r = el.getBoundingClientRect();
          if (r.width === 0 || r.bottom < 0 || r.top > innerHeight || el.closest('.lp-nav, .story-stage, [aria-hidden=true]')) continue;
          for (const hd of heads) {
            const b = hd.getBoundingClientRect();
            const x0 = Math.max(r.left, b.left), x1 = Math.min(r.right, b.right), y0 = Math.max(r.top, b.top), y1 = Math.min(r.bottom, b.bottom);
            if (x1 - x0 < 4 || y1 - y0 < 2) continue;
            // text may scroll under the header only if the header is opaque there
            const top = document.elementFromPoint((x0 + x1) / 2, (y0 + y1) / 2);
            const alpha = Number((getComputedStyle(hd).backgroundColor.match(/[\d.]+/g) ?? [])[3] ?? 1);
            if (!(top && hd.contains(top)) || alpha < 0.9) out.push(`${el.tagName} "${el.textContent.trim().slice(0, 24)}"`);
          }
        }
        return out;
      });
      expect(bad.length === 0, `${w}px at ${y}: text shows through the sticky header: ${bad.join(', ')}`);
    }
    await ctx.close();
  }
});

await step('every landing locale: lang and dir, no overflow, one-line nav, motion switch', async () => {
  const built = LOCALES.filter((l) => existsSync(join(dist, l.slug, 'index.html')));
  expect(built.length >= 10, `expected the landing in every locale, found ${built.map((l) => l.code).join(',')}`);
  for (const loc of built) {
    for (const [w, h] of [[320, 640], [390, 844], [1440, 900]]) {
      const c = await browser.newContext({ viewport: { width: w, height: h }, isMobile: w < 700, hasTouch: w < 700, locale: 'en-GB', timezoneId: 'UTC' });
      const p = await c.newPage();
      const errs = [];
      p.on('pageerror', (e) => errs.push(e.message));
      p.on('console', (m) => m.type() === 'error' && !external(m.location().url ?? '') && errs.push(m.text()));
      await p.route((u) => u.hostname !== '127.0.0.1', (route) => route.abort());
      await p.goto(`${SITE}/${loc.slug ? `${loc.slug}/` : ''}`);
      await p.waitForTimeout(500);
      const m = await p.evaluate(() => {
        const nav = [...document.querySelectorAll('.lp-nav .nav > a, .lp-nav .lang-btn')].filter((a) => a.offsetParent).map((a) => { const r = a.getBoundingClientRect(); return { t: a.textContent.trim(), h: r.height, r: r.right }; });
        const bar = document.querySelector('.lp-nav-bar').getBoundingClientRect();
        const clipped = [...document.querySelectorAll('.btn-pill, .lp-nav .btn, .badge, .lang-btn')].filter((e) => e.offsetParent && e.scrollWidth > e.clientWidth + 1).map((e) => e.textContent.trim());
        return { lang: document.documentElement.lang, dir: document.documentElement.dir, over: document.documentElement.scrollWidth - innerWidth, nav, barRight: bar.right, barLeft: bar.left, clipped, left: document.querySelectorAll('[data-locale]').length };
      });
      const tag = `${loc.code} at ${w}px`;
      expect(m.lang === loc.lang && m.dir === loc.dir, `${tag}: html lang/dir are ${m.lang}/${m.dir}`);
      expect(m.over <= 0, `${tag}: horizontal overflow of ${m.over}px`);
      expect(m.nav.every((a) => a.h < 50 && a.r <= m.barRight + 1 && a.r >= m.barLeft - 1), `${tag}: nav wraps or leaves its bar: ${JSON.stringify(m.nav)}`);
      expect(m.clipped.length === 0, `${tag}: clipped labels: ${m.clipped.join(', ')}`);
      if (!loc.hidden) expect(m.left >= 2 * (built.filter((l) => !l.hidden).length), `${tag}: the language list is missing entries`);
      if (w === 1440) {
        // The motion switch behaves the same in every language: label, toggle, remembered, and gone with reduced motion.
        const btn = p.locator('[data-motion]');
        expect((await btn.getAttribute('aria-pressed')) === 'false', `${tag}: motion switch should start unpressed`);
        // Keyboard, not a pointer click: the hero is still animating and Playwright waits for a stable box.
        await btn.focus();
        await p.keyboard.press('Enter');
        expect((await btn.getAttribute('aria-pressed')) === 'true', `${tag}: motion switch should toggle`);
        expect(await p.evaluate(() => document.documentElement.classList.contains('motion-off') && localStorage.getItem('socialos_landing_motion') === 'off'), `${tag}: motion-off class and saved preference`);
        if (!loc.hidden) await p.screenshot({ path: join(out, `locale-${loc.code}.png`) });
      }
      expect(errs.length === 0, `${tag}: console problems: ${errs.join('; ')}`);
      await c.close();
    }
    const r = await browser.newContext({ viewport: { width: 1440, height: 900 }, reducedMotion: 'reduce', locale: 'en-GB' });
    const rp = await r.newPage();
    await rp.route((u) => u.hostname !== '127.0.0.1', (route) => route.abort());
    await rp.goto(`${SITE}/${loc.slug ? `${loc.slug}/` : ''}`);
    expect(await rp.locator('[data-motion]').isHidden(), `${loc.code}: the motion switch is not offered with reduced motion`);
    expect(await rp.evaluate(() => document.documentElement.classList.contains('motion-off')), `${loc.code}: reduced motion starts paused`);
    await r.close();
  }
});

await step('first-visit suggestion: offered, never redirects, remembered', async () => {
  const c = await browser.newContext({ viewport: { width: 390, height: 844 }, locale: 'de-DE' });
  const p = await c.newPage();
  await p.route((u) => u.hostname !== '127.0.0.1', (route) => route.abort());
  await p.goto(`${SITE}/`);
  const bar = p.locator('.lang-suggest');
  await bar.waitFor({ state: 'visible', timeout: 5000 });
  expect(new URL(p.url()).pathname === BASE, 'a German browser must stay on the English page (no redirect)');
  expect((await bar.getAttribute('lang')) === 'de' && /Deutsch/.test(await bar.innerText()), 'the suggestion is written in German');
  const over = await p.evaluate(() => document.documentElement.scrollWidth - innerWidth);
  expect(over <= 0, `the suggestion causes horizontal overflow of ${over}px`);
  await p.locator('.lang-suggest button').click();
  expect((await bar.count()) === 0, 'dismissing removes the suggestion');
  await p.reload();
  await p.waitForTimeout(600);
  expect((await p.locator('.lang-suggest').count()) === 0, 'a dismissed suggestion does not come back');
  expect(await p.evaluate(() => localStorage.getItem('steerpost_lang_suggest') === 'dismissed' && localStorage.getItem('steerpost_locale') === null), 'dismissing uses its own key and leaves the app locale alone');
  // Following the link goes to the German page and is remembered for the next visit to the root.
  await p.evaluate(() => localStorage.clear());
  await p.reload();
  await p.locator('.lang-suggest a').click();
  await p.waitForURL(new RegExp(`${BASE}de/`));
  expect((await p.evaluate(() => localStorage.getItem('steerpost_locale'))) === 'de', 'the choice is stored');
  await c.close();
  // English and Ukrainian browsers get no suggestion.
  for (const locale of ['en-US', 'uk-UA']) {
    const e = await browser.newContext({ viewport: { width: 1280, height: 800 }, locale });
    const ep = await e.newPage();
    await ep.route((u) => u.hostname !== '127.0.0.1', (route) => route.abort());
    await ep.goto(`${SITE}/`);
    await ep.waitForTimeout(600);
    expect((await ep.locator('.lang-suggest').count()) === 0, `${locale} should not be offered another language`);
    await e.close();
  }
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
  // The renderer is served from this site, so the diagram is drawn even with every other host blocked.
  await page.locator('pre.mermaid svg').first().waitFor({ state: 'attached', timeout: 20_000 });
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
  // Reset asks for confirmation since the UI audit (#140).
  await page.getByRole('button', { name: 'Reset demo', exact: true }).click();
  await page.waitForURL(/\/demo\/dashboard\/?$/, { timeout: 15_000 });
  await visible(page.getByText('Release 2.5 teaser'));
  await page.goto(`${SITE}/demo/calendar/`);
  await visible(page.getByRole('heading', { name: /\d{4}/ }));
  await monthGoto();
  await page.waitForTimeout(500);
  expect((await page.getByRole('link', { name: new RegExp(TITLE) }).count()) === 0, 'the smoke post should be gone after Reset');
});

await step('every demo screen shows text, never a raw message key (en, ru)', async () => {
  // A route whose scope lacks a namespace, or text shown before a locale's bundles arrived, prints `posts.untitled`.
  const fresh = await browser.newContext({ viewport: { width: 1280, height: 800 }, locale: 'en-GB', timezoneId: 'UTC' });
  const p = await fresh.newPage();
  const errs = [];
  p.on('pageerror', (e) => errs.push(e.message));
  p.on('console', (m) => m.type() === 'error' && !external(m.location().url ?? '') && errs.push(m.text()));
  await p.goto(`${SITE}/demo/posts/`);
  await p.getByRole('link', { name: /Release 2.5 teaser/ }).first().waitFor({ state: 'visible' });
  const detail = new URL(await p.getByRole('link', { name: /Release 2.5 teaser/ }).first().getAttribute('href'), p.url()).href;
  const open = (route) => p.goto(route.startsWith('http') ? route : `${SITE}/demo${route}/`);
  const findings = await scanForRawKeys(p, [...APP_ROUTES, ...AUTH_ROUTES, detail], { open });
  await fresh.close();
  expect(findings.length === 0, `raw message keys on screen: ${JSON.stringify(findings)}`);
  expect(errs.length === 0, `console problems: ${errs.join('; ')}`);
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
