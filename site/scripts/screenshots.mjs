#!/usr/bin/env node
/**
 * Captures the product screenshots used on the landing page, in light and dark, from the real app
 * running as the browser-only demo.
 *
 *   (cd ../frontend && npm run build:demo) && npm run screenshots
 *
 * Output: src/assets/screens/<name>-<light|dark>.png (committed; re-run when the UI changes).
 * The demo is served at /SMSI/demo/ from ../frontend/out, with the clock fixed so the data is stable.
 */
import { spawnSync } from 'node:child_process';
import { mkdirSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { launch } from './lib.mjs';
import { startServer } from './serve.mjs';

const here = dirname(fileURLToPath(import.meta.url));
const out = resolve(here, process.argv[2] ?? '../src/assets/screens');
const demoDir = resolve(here, process.env.DEMO_DIR ?? '../../frontend/out');
const PORT = 4181;
const ORIGIN = `http://127.0.0.1:${PORT}/SMSI/demo`;
// Mid-month, mid-week: the calendar looks lived in and "now" is stable between runs.
const NOW = new Date('2026-10-14T10:20:00Z');
const VIEWPORT = { width: 1280, height: 800 };

mkdirSync(out, { recursive: true });
const server = await startServer({ dir: demoDir, port: PORT, base: '/SMSI/demo/' });
const browser = await launch();

try {
  for (const scheme of ['light', 'dark']) {
    const ctx = await browser.newContext({ viewport: VIEWPORT, colorScheme: scheme, locale: 'en-GB', timezoneId: 'UTC', deviceScaleFactor: 2 });
    const page = await ctx.newPage();
    await page.clock.setFixedTime(NOW);
    const errors = [];
    page.on('pageerror', (e) => errors.push(e.message));
    page.on('console', (m) => m.type() === 'error' && errors.push(m.text()));
    const shot = async (name) => {
      await page.waitForTimeout(300);
      await page.screenshot({ path: join(out, `${name}-${scheme}.png`) });
      console.log(`${name}-${scheme}.png`);
    };
    const open = async (path, ready) => {
      await page.goto(`${ORIGIN}${path}`);
      await page.getByRole(ready.role, { name: ready.name, exact: ready.exact ?? false }).first().waitFor();
    };

    await open('/dashboard/', { role: 'heading', name: 'Upcoming' });
    await shot('dashboard');

    await open('/compose/', { role: 'group', name: 'Publish to' });
    await page.getByLabel('Title (optional)').fill('Release 2.5 teaser');
    await page.getByRole('button', { name: /Jordan Lee/ }).click();
    await page.getByRole('button', { name: /Studio Updates/ }).click();
    await page.getByLabel('Post content').fill(
      'Release 2.5 lands next week. A small change that has been on your wishlist for a while: scheduling that understands your timezone, finally.\n\nNotes and a short walkthrough are coming Thursday.',
    );
    await page.getByRole('tab', { name: /Telegram/ }).click();
    await page.getByLabel(/Telegram content for/).fill('Release 2.5 lands next week: scheduling that understands your timezone. Walkthrough on Thursday.');
    await page.getByRole('tab', { name: 'All platforms' }).click();
    await page.locator('#sched-date').fill('2026-10-15');
    await page.locator('#sched-time').fill('09:00');
    await page.locator('#sched-time').blur();
    await shot('compose');

    await open('/calendar/', { role: 'heading', name: 'October 2026' });
    await shot('calendar');

    await open('/accounts/', { role: 'heading', name: 'LinkedIn', exact: true });
    // Scroll past LinkedIn so the shot ends with the networks that are not available yet.
    await page.evaluate(() => window.scrollTo(0, 395));
    await shot('accounts');

    await open('/developer/mcp/', { role: 'heading', name: 'Connect an AI agent' });
    await page.getByLabel('Connection name').fill('Claude Desktop');
    await page.getByLabel(/Schedule/).check();
    await page.getByRole('button', { name: 'Create connection' }).click();
    await page.getByTestId('mcp-created').waitFor();
    await shot('mcp');

    if (errors.length) console.warn(`console errors in ${scheme}:\n${errors.join('\n')}`);
    await ctx.close();
  }
} finally {
  await browser.close();
  server.close();
}

// Optional: squeeze the PNGs to 256 colours (UI screenshots have few colours; ~2.5x smaller, visually identical).
const squeeze = `
import sys, glob
from PIL import Image
for f in glob.glob(sys.argv[1] + '/*.png'):
    Image.open(f).convert('RGB').quantize(colors=256, method=Image.Quantize.MEDIANCUT, dither=Image.Dither.NONE).save(f, optimize=True)
`;
const r = spawnSync('python3', ['-I', '-c', squeeze, out], { encoding: 'utf8' });
console.log(r.status === 0 ? 'optimised PNGs (256 colours)' : 'skipped PNG optimisation (python3 with Pillow not available)');
