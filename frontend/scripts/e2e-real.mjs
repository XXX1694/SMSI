#!/usr/bin/env node
/**
 * Browser end-to-end run against a REAL SocialOS stack (API + worker with
 * SOCIAL_MOCK_PROVIDERS=true, frontend on WEB_URL). Fails on any 5xx, any
 * unexpected 4xx from /api, or any console error. Screenshots → ./screenshots/real-*.png
 *   WEB_URL=http://localhost:3000 node scripts/e2e-real.mjs
 */
import { existsSync, mkdirSync, readdirSync } from 'node:fs';
import { chromium } from 'playwright-core';

const BASE = process.env.WEB_URL ?? 'http://localhost:3000';
const OUT = new URL('../screenshots/', import.meta.url).pathname;
mkdirSync(OUT, { recursive: true });

function findChromium() {
  if (process.env.CHROMIUM_PATH) return process.env.CHROMIUM_PATH;
  const root = '/opt/pw-browsers';
  for (const d of existsSync(root) ? readdirSync(root) : []) {
    const p = `${root}/${d}/chrome-linux/chrome`;
    if (d.startsWith('chromium-') && existsSync(p)) return p;
  }
  return undefined;
}

const problems = [];
const browser = await chromium.launch({ executablePath: findChromium() });
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
// HTTP failures are checked precisely below; the browser's generic "Failed to load resource" console line adds nothing.
page.on('console', (m) => { if (m.type() === 'error' && !m.text().startsWith('Failed to load resource')) problems.push(`console: ${m.text()}`); });
page.on('requestfailed', (r) => { if (!r.failure()?.errorText.includes('ERR_ABORTED')) problems.push(`requestfailed ${r.url()} ${r.failure()?.errorText}`); });
page.on('pageerror', (e) => problems.push(`pageerror: ${e.message}`));
page.on('response', (r) => {
  const u = new URL(r.url());
  if (!u.pathname.startsWith('/api/')) { if (r.status() >= 400) problems.push(`asset ${u.pathname} → ${r.status()}`); return; }
  // 401 on /me before login is expected.
  if (r.status() >= 500 || (r.status() >= 400 && !(r.status() === 401 && u.pathname.endsWith('/me')))) {
    problems.push(`${r.request().method()} ${u.pathname} → ${r.status()}`);
  }
});
const shot = (name) => page.screenshot({ path: `${OUT}real-${name}.png`, fullPage: true });
const step = (m) => console.log(`✓ ${m}`);

try {
  await page.goto(`${BASE}/register`);
  await page.getByLabel(/name/i).first().fill('Browser User');
  await page.getByLabel(/email/i).fill(`browser+${Date.now()}@example.com`);
  await page.getByLabel(/password/i).first().fill('correct horse battery');
  await page.getByRole('button', { name: /create account|register|sign up/i }).click();
  await page.waitForURL(/dashboard/, { timeout: 15000 });
  step('registered and landed on dashboard');

  for (const hint of ['linkedin', 'telegram']) {
    await page.goto(`${BASE}/api/v1/social/mock/connect?redirect=/accounts&account=${hint}`);
    await page.waitForURL(/accounts/, { timeout: 15000 });
  }
  await page.waitForLoadState('networkidle');
  await shot('accounts');
  step('connected two accounts');

  await page.goto(`${BASE}/compose`);
  await page.waitForLoadState('networkidle');
  await page.locator('textarea').first().fill('Hello from the SocialOS browser e2e run');
  const chips = page.getByRole('checkbox').or(page.locator('[aria-pressed]'));
  const n = await chips.count();
  for (let i = 0; i < n; i++) await chips.nth(i).click();
  await shot('compose');
  await page.getByRole('button', { name: /save draft/i }).click();
  await page.waitForTimeout(1500);
  step(`composer saved a draft (${n} account toggles)`);

  for (const p of ['dashboard', 'posts', 'calendar', 'media', 'analytics', 'settings', 'developer', 'developer/mcp']) {
    await page.goto(`${BASE}/${p}`);
    await page.waitForLoadState('networkidle');
    await shot(p.replace('/', '-'));
  }
  step('visited every page');
} catch (e) {
  problems.push(`flow: ${e.message}`);
  await shot('failure');
} finally {
  await browser.close();
}

if (problems.length) {
  console.error('\nPROBLEMS:\n' + [...new Set(problems)].join('\n'));
  process.exit(1);
}
console.log('\nBROWSER E2E PASSED');
