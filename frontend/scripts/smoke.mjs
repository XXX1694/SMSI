#!/usr/bin/env node
/**
 * Playwright smoke run against the mock API. Requires `npm run build` first.
 *   CHROMIUM_PATH=/opt/pw-browsers/chromium-1194/chrome-linux/chrome node scripts/smoke.mjs
 * Screenshots land in ./screenshots.
 */
import { spawn } from 'node:child_process';
import { existsSync, mkdirSync, readdirSync } from 'node:fs';
import { chromium } from 'playwright-core';

const API_PORT = 8080;
const WEB_PORT = 3100;
const BASE = `http://localhost:${WEB_PORT}`;
const OUT = new URL('../screenshots/', import.meta.url).pathname;
mkdirSync(OUT, { recursive: true });

function findChromium() {
  if (process.env.CHROMIUM_PATH) return process.env.CHROMIUM_PATH;
  const root = '/opt/pw-browsers';
  if (existsSync(root)) {
    for (const d of readdirSync(root).filter((x) => x.startsWith('chromium-'))) {
      const p = `${root}/${d}/chrome-linux/chrome`;
      if (existsSync(p)) return p;
    }
  }
  return undefined;
}

const children = [];
function run(cmd, args, env) {
  const c = spawn(cmd, args, { env: { ...process.env, ...env }, stdio: 'ignore', detached: true });
  children.push(c);
  return c;
}
async function waitFor(url) {
  for (let i = 0; i < 60; i++) {
    try { if ((await fetch(url)).status < 500) return; } catch { /* retry */ }
    await new Promise((r) => setTimeout(r, 500));
  }
  throw new Error(`timeout waiting for ${url}`);
}
const stop = () => children.forEach((c) => { try { process.kill(-c.pid); } catch { /* already gone */ } });

let failures = 0;
const check = (ok, msg) => { console.log(`${ok ? 'PASS' : 'FAIL'}  ${msg}`); if (!ok) failures++; };

try {
  run('node', ['scripts/mock-api.mjs'], { PORT: String(API_PORT) });
  run('npx', ['next', 'start', '-p', String(WEB_PORT)], {});
  await waitFor(`http://localhost:${API_PORT}/api/v1/health`);
  await waitFor(`${BASE}/login`);

  const browser = await chromium.launch({ executablePath: findChromium(), args: ['--no-sandbox'] });
  const ctx = await browser.newContext({ viewport: { width: 1280, height: 860 } });
  const page = await ctx.newPage();
  const errors = [];
  page.on('pageerror', (e) => errors.push(e.message));
  page.on('console', (m) => { if (m.type() === 'error') errors.push(m.text()); });
  const shot = (name) => page.screenshot({ path: `${OUT}${name}.png`, fullPage: true });

  // Unauthenticated -> redirected to login
  await page.goto(`${BASE}/dashboard`);
  await page.waitForURL(/\/login/);
  check(true, 'auth guard redirects to /login');
  await shot('00-login');

  await page.getByLabel('Email').fill('demo@example.com');
  await page.getByLabel('Password').fill('demo12345');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await page.waitForURL(/\/dashboard/);
  await page.getByText('Connected accounts').waitFor();
  check(await page.getByText('Published this month').isVisible(), 'dashboard stats visible');
  await shot('01-dashboard');

  await page.goto(`${BASE}/accounts`);
  await page.getByRole('heading', { name: 'LinkedIn' }).waitFor();
  check((await page.getByText('Not available').count()) >= 7, 'unsupported providers marked "Not available"');
  await shot('02-accounts');

  await page.goto(`${BASE}/compose`);
  await page.getByRole('button', { name: /Alex Morgan/ }).click();
  await page.getByRole('button', { name: /Steerpost Demo Channel/ }).click();
  await page.getByLabel('Post content').fill('Shipping Steerpost today. Write once, publish everywhere.');
  await page.getByRole('tab', { name: /LinkedIn/ }).click();
  await page.getByRole('tab', { name: 'All platforms' }).click();
  await shot('03-composer');
  // Schedule path with a future date
  const d = new Date(Date.now() + 3 * 86400000).toISOString().slice(0, 10);
  await page.getByLabel('Date').fill(d);
  await page.getByRole('button', { name: 'Schedule' }).click();
  await page.waitForURL(/\/posts\/[0-9a-f-]{36}/);
  await page.getByText('Attempt history').waitFor();
  check(await page.getByText('Scheduled', { exact: true }).first().isVisible(), 'compose -> schedule -> post detail');
  await shot('04-post-detail');

  // Publish-now flow with simulated failure
  await page.goto(`${BASE}/compose`);
  await page.getByRole('button', { name: /Mock Account/ }).click();
  await page.getByLabel('Post content').fill('FAIL this one');
  await page.getByRole('button', { name: 'Publish now' }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'Publish now' }).click();
  await page.waitForURL(/\/posts\/[0-9a-f-]{36}/);
  await page.getByText('Provider rejected the request').first().waitFor({ timeout: 8000 });
  check(true, 'failed publish shows per-target error');
  await shot('05-post-failed');

  await page.goto(`${BASE}/posts`);
  await page.getByText('Launch announcement').waitFor();
  await shot('06-posts');
  await page.goto(`${BASE}/calendar`);
  await page.getByRole('heading', { level: 2 }).first().waitFor();
  await shot('07-calendar');
  await page.getByRole('button', { name: 'week' }).click();
  await shot('07b-calendar-week');
  await page.goto(`${BASE}/media`);
  await page.getByText('launch-banner.png').waitFor();
  await shot('08-media');
  await page.goto(`${BASE}/analytics`);
  await page.getByText('Impressions').waitFor();
  await shot('09-analytics');

  await page.goto(`${BASE}/developer`);
  await page.getByText('CI reader').first().waitFor();
  await shot('10-developer');
  await page.getByRole('button', { name: 'Create key' }).click();
  await page.getByLabel('Name').fill('Smoke key');
  const dangerous = page.getByRole('checkbox', { name: /Publish immediately/ });
  check(!(await dangerous.isChecked()), 'dangerous scope unchecked by default');
  await shot('11-developer-create');
  await page.getByRole('dialog').getByRole('button', { name: 'Create key' }).click();
  const raw = await page.getByTestId('raw-key').textContent();
  check(/^sk_live_/.test(raw ?? ''), 'raw key revealed once');
  await shot('12-developer-rawkey');
  await page.getByRole('button', { name: 'I have saved it' }).click();

  await page.goto(`${BASE}/developer/mcp`);
  await page.getByLabel('Connection name').fill('Smoke agent');
  await page.getByRole('button', { name: 'Create connection' }).click();
  await page.getByTestId('mcp-created').waitFor();
  await shot('13-mcp-created');
  check((await page.getByTestId('mcp-created').textContent())?.includes('Bearer sk_live_') ?? false, 'MCP config includes key');
  await page.getByRole('button', { name: 'I have saved it' }).click();
  await shot('14-mcp-list');

  await page.goto(`${BASE}/settings`);
  await page.getByLabel('Theme').selectOption('dark');
  await shot('15-settings-dark');
  await page.goto(`${BASE}/dashboard`);
  await page.getByText('Connected accounts').waitFor();
  await shot('16-dashboard-dark');

  // Mobile
  const mctx = await browser.newContext({ viewport: { width: 390, height: 800 } });
  const m = await mctx.newPage();
  await m.goto(`${BASE}/login`);
  await m.getByLabel('Email').fill('demo@example.com');
  await m.getByLabel('Password').fill('demo12345');
  await m.getByRole('button', { name: 'Sign in' }).click();
  await m.getByText('Connected accounts').waitFor();
  await m.screenshot({ path: `${OUT}20-mobile-dashboard.png`, fullPage: true });
  await m.getByRole('button', { name: 'Open menu' }).click();
  await m.screenshot({ path: `${OUT}21-mobile-menu.png` });
  await m.goto(`${BASE}/compose`);
  await m.getByLabel('Post content').waitFor();
  await m.screenshot({ path: `${OUT}22-mobile-compose.png`, fullPage: true });
  const overflow = await m.evaluate(() => document.documentElement.scrollWidth > window.innerWidth);
  check(!overflow, 'no horizontal overflow on mobile compose');

  check(errors.filter((e) => !/favicon|Failed to load resource/.test(e)).length === 0, `no console/page errors ${errors.length ? JSON.stringify(errors.slice(0, 3)) : ''}`);
  await browser.close();
} catch (e) {
  console.error(e);
  failures++;
} finally {
  stop();
}
console.log(failures === 0 ? 'SMOKE OK' : `SMOKE FAILED (${failures})`);
process.exit(failures === 0 ? 0 : 1);
