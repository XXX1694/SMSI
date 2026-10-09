#!/usr/bin/env node
/**
 * Records the hero video of the landing page from the real app running as the browser-only demo:
 * compose a post, schedule it, then approve an agent's request. Real footage, no mock-ups.
 *
 *   (cd ../frontend && npm run build:demo) && npm run record
 *
 * Output: src/assets/video/hero-<light|dark>.{webm,mp4} and hero-<light|dark>.jpg (poster), all committed.
 * Needs ffmpeg (libvpx-vp9, libx264). The recording adds one thing the app does not draw: a small cursor dot,
 * because browser video capture does not include the pointer.
 *
 * Frames are rendered, not screen-recorded: Playwright's recordVideo captures about 25 fps and drops frames under load,
 * which shows as the cursor stalling and then catching up. Here virtual time advances one frame at a time (fake
 * timers, and CSS animations/transitions paused and stepped by hand), the cursor position comes from an eased path at
 * t = frame / FPS, and every frame is a screenshot. The result has no dropped frames whatever the machine's speed.
 */
import { spawnSync } from 'node:child_process';
import { mkdirSync, mkdtempSync, readdirSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { launch } from './lib.mjs';
import { startServer } from './serve.mjs';

const here = dirname(fileURLToPath(import.meta.url));
const out = resolve(here, process.argv[2] ?? '../src/assets/video');
const demoDir = resolve(here, process.env.DEMO_DIR ?? '../../frontend/out');
const PORT = 4182;
const ORIGIN = `http://127.0.0.1:${PORT}/steerpost/demo`;
const SIZE = { width: 1280, height: 800 };
const NOW = new Date('2026-10-14T10:20:00Z');
const FPS = 60;
const DT = 1000 / FPS;
const OUT_FPS = Number(process.env.OUT_FPS ?? 60);

const cursor = `
  try { localStorage.setItem('socialos_mail_notice_dismissed', '1'); } catch {}
  const dot = document.createElement('div');
  dot.setAttribute('aria-hidden', 'true');
  dot.style.cssText = 'position:fixed;left:0;top:0;width:18px;height:18px;margin:-9px 0 0 -9px;border-radius:50%;background:rgba(8,107,129,.6);border:2px solid #fff;box-shadow:0 2px 8px rgba(0,0,0,.35);z-index:2147483647;pointer-events:none;translate:980px 120px;transition:scale .12s';
  const mount = () => document.documentElement.append(dot);
  document.readyState === 'loading' ? addEventListener('DOMContentLoaded', mount) : mount();
  addEventListener('mousemove', (e) => { dot.style.translate = e.clientX + 'px ' + e.clientY + 'px'; }, true);
  addEventListener('mousedown', () => { dot.style.scale = '.75'; }, true);
  addEventListener('mouseup', () => { dot.style.scale = '1'; }, true);
`;

/** Pauses every CSS animation/transition the moment it appears and moves it forward by hand, one frame at a time. */
const stepper = `
  window.__step = (dt) => {
    for (const a of document.getAnimations()) {
      if (!a.__held) { a.__held = true; a.pause(); }
      const end = a.effect?.getComputedTiming().endTime;
      a.currentTime = (a.currentTime ?? 0) + dt;
      if (Number.isFinite(end) && a.currentTime >= end) a.finish();
    }
  };
`;

let at = { x: 980, y: 120 };
let bend = 1;
let debt = 0;
let frame = 0;
let framesDir = '';
const easeInOut = (t) => (t < 0.5 ? 2 * t * t : 1 - (-2 * t + 2) ** 2 / 2);

/** Advance virtual time by one frame and save it. */
async function shoot(page) {
  await page.clock.runFor(DT);
  await page.evaluate((dt) => window.__step(dt), DT);
  await page.screenshot({ path: join(framesDir, `${String(frame++).padStart(5, '0')}.png`), caret: 'initial' });
}

/** Let `ms` of virtual time pass. Sub-frame remainders carry over, so typing delays of 24 ms still add up. */
async function wait(page, ms) {
  debt += ms;
  while (debt >= DT) {
    debt -= DT;
    await shoot(page);
  }
}

/** Frames until the locator is visible: the app needs virtual time to pass for timers and effects to run. */
async function until(page, locator) {
  for (let i = 0; i < 6 * FPS; i++) {
    if (await locator.first().isVisible()) return;
    await shoot(page);
  }
  throw new Error('timed out waiting for a locator in the recording');
}

async function type(page, text, delay) {
  for (const ch of text) {
    await page.keyboard.type(ch);
    await wait(page, delay);
  }
}

/** Glide like a person: one eased quadratic arc; the position at each frame is computed, not captured. */
async function glide(page, x, y) {
  const { x: x0, y: y0 } = at;
  const dist = Math.hypot(x - x0, y - y0);
  if (dist >= 1) {
    const frames = Math.round(Math.min(1500, 450 + dist * 1.1) / DT);
    bend = -bend;
    const cx = (x0 + x) / 2 - ((y - y0) / dist) * dist * 0.1 * bend;
    const cy = (y0 + y) / 2 + ((x - x0) / dist) * dist * 0.1 * bend;
    for (let i = 1; i <= frames; i++) {
      const e = easeInOut(i / frames);
      const u = 1 - e;
      await page.mouse.move(u * u * x0 + 2 * u * e * cx + e * e * x, u * u * y0 + 2 * u * e * cy + e * e * y);
      await shoot(page);
    }
  }
  at = { x, y };
}

/** Move like a person: a short eased glide, then click. */
async function clickOn(page, locator) {
  const el = locator.first();
  await el.scrollIntoViewIfNeeded();
  const box = await el.boundingBox();
  await glide(page, box.x + box.width / 2, box.y + box.height / 2);
  await wait(page, 140);
  await page.mouse.down();
  await wait(page, 70);
  await page.mouse.up();
}

async function record(browser, scheme, tmp) {
  framesDir = join(tmp, scheme);
  mkdirSync(framesDir);
  frame = 0;
  debt = 0;
  at = { x: 980, y: 120 };
  const ctx = await browser.newContext({ viewport: SIZE, colorScheme: scheme, locale: 'en-GB', timezoneId: 'UTC' });
  await ctx.addInitScript(cursor);
  await ctx.addInitScript(stepper);
  const page = await ctx.newPage();
  await page.clock.install({ time: NOW });
  await page.mouse.move(980, 120);

  await page.goto(`${ORIGIN}/dashboard/`);
  await until(page, page.getByRole('heading', { name: 'Upcoming' }));
  await wait(page, 1000);

  await clickOn(page, page.getByRole('link', { name: 'Compose', exact: true }));
  await until(page, page.getByRole('group', { name: 'Publish to' }));
  await wait(page, 500);
  await clickOn(page, page.getByLabel('Title (optional)'));
  await type(page, 'Release 2.5 teaser', 38);
  await clickOn(page, page.getByRole('button', { name: /Jordan Lee/ }));
  await clickOn(page, page.getByRole('button', { name: /Studio Updates/ }));
  await clickOn(page, page.getByLabel('Post content'));
  await type(page, 'Release 2.5 lands next week. Scheduling that understands your timezone, finally.', 24);
  await wait(page, 500);
  await page.locator('#sched-date').fill('2026-10-15');
  await page.locator('#sched-time').fill('09:00');
  await page.locator('#sched-time').blur();
  await wait(page, 700);
  // Poster: the whole form from the Title down, so scroll to the top for the shot and put the page back.
  const scrolled = await page.evaluate(() => {
    const y = window.scrollY;
    window.scrollTo(0, 0);
    return y;
  });
  const poster = await page.screenshot({ type: 'jpeg', quality: 88 });
  await page.evaluate((y) => window.scrollTo(0, y), scrolled);
  await clickOn(page, page.getByRole('button', { name: 'Schedule', exact: true }));
  await until(page, page.getByText('Scheduled', { exact: true }));
  await wait(page, 1100);

  await clickOn(page, page.getByRole('link', { name: 'Approvals' }));
  await until(page, page.getByRole('button', { name: /^Approve/ }));
  await wait(page, 1000);
  await clickOn(page, page.getByRole('button', { name: /^Approve/ }).first());
  await wait(page, 1300);

  await ctx.close();
  return { dir: framesDir, poster };
}

function ffmpeg(args) {
  const r = spawnSync('ffmpeg', ['-y', '-loglevel', 'error', ...args], { encoding: 'utf8' });
  if (r.status !== 0) throw new Error(`ffmpeg ${args.join(' ')}\n${r.stderr}`);
}

/** Loop-friendly: starts on the dashboard, ends on the approved state. Encoded at OUT_FPS (default 60 = every rendered frame). */
function transcode(dir, poster, name) {
  const base = join(out, name);
  const common = ['-framerate', String(FPS), '-i', join(dir, '%05d.png'), '-an', '-vf', `fps=${OUT_FPS},scale=1280:-2:flags=lanczos`];
  ffmpeg([...common, '-c:v', 'libvpx-vp9', '-b:v', '0', '-crf', '46', '-row-mt', '1', '-pix_fmt', 'yuv420p', `${base}.webm`]);
  ffmpeg([...common, '-c:v', 'libx264', '-preset', 'slow', '-crf', '29', '-pix_fmt', 'yuv420p', '-movflags', '+faststart', `${base}.mp4`]);
  writeFileSync(`${base}.jpg`, poster);
}

mkdirSync(out, { recursive: true });
const tmp = mkdtempSync(join(tmpdir(), 'hero-rec-'));
const server = await startServer({ dir: demoDir, port: PORT, base: '/steerpost/demo/' });
const browser = await launch();
try {
  for (const scheme of ['light', 'dark']) {
    const { dir, poster } = await record(browser, scheme, tmp);
    transcode(dir, poster, `hero-${scheme}`);
    console.log(`hero-${scheme}.{webm,mp4,jpg}: ${readdirSync(dir).length} frames`);
  }
} finally {
  await browser.close();
  server.close();
  rmSync(tmp, { recursive: true, force: true });
}
