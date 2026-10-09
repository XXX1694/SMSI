#!/usr/bin/env node
/**
 * Records the hero video of the landing page from the real app running as the browser-only demo:
 * compose a post, schedule it, then approve an agent's request. Real footage, no mock-ups.
 *
 *   (cd ../frontend && npm run build:demo) && npm run record
 *
 * Output: src/assets/video/hero-<light|dark>.{webm,mp4} and hero-<light|dark>.jpg (poster), all committed.
 * Needs ffmpeg (libvpx-vp9, libx264). The recording adds one thing the app does not draw: a small cursor dot,
 * because browser video capture does not include the pointer. Playwright captures at 25 fps, so the output stays at 25
 * (resampling to 30 repeats every sixth frame and the cursor visibly stutters) and the dot is moved by timed, eased
 * glides instead of a burst of mouse events.
 */
import { spawnSync } from 'node:child_process';
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
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
const pause = (page, ms) => page.waitForTimeout(ms);

const cursor = `
  try { localStorage.setItem('socialos_mail_notice_dismissed', '1'); } catch {}
  const dot = document.createElement('div');
  dot.setAttribute('aria-hidden', 'true');
  dot.style.cssText = 'position:fixed;left:0;top:0;width:18px;height:18px;margin:-9px 0 0 -9px;border-radius:50%;background:rgba(8,107,129,.6);border:2px solid #fff;box-shadow:0 2px 8px rgba(0,0,0,.35);z-index:2147483647;pointer-events:none;transform:translate(980px,120px);transition:scale .12s';
  const mount = () => document.documentElement.append(dot);
  document.readyState === 'loading' ? addEventListener('DOMContentLoaded', mount) : mount();
  addEventListener('mousemove', (e) => { dot.style.transform = 'translate(' + e.clientX + 'px,' + e.clientY + 'px)'; }, true);
  addEventListener('mousedown', () => { dot.style.scale = '.75'; }, true);
  addEventListener('mouseup', () => { dot.style.scale = '1'; }, true);
`;

let at = { x: 980, y: 120 };
let bend = 1;
const easeInOut = (t) => (t < 0.5 ? 2 * t * t : 1 - (-2 * t + 2) ** 2 / 2);

/**
 * Glide like a person: one eased quadratic arc, paced by the wall clock. Mouse events are timed (not a burst of
 * `steps`, which land in one or two captured frames and read as a jump), so the dot covers about the same distance
 * in every captured frame whatever the capture rate is.
 */
async function glide(page, x, y) {
  const { x: x0, y: y0 } = at;
  const dist = Math.hypot(x - x0, y - y0);
  if (dist < 1) return;
  const dur = Math.min(1500, 450 + dist * 1.1);
  bend = -bend;
  const cx = (x0 + x) / 2 - ((y - y0) / dist) * dist * 0.1 * bend;
  const cy = (y0 + y) / 2 + ((x - x0) / dist) * dist * 0.1 * bend;
  const t0 = Date.now();
  for (let t = 0; t < 1; ) {
    t = Math.min(1, (Date.now() - t0) / dur);
    const e = easeInOut(t);
    const u = 1 - e;
    await page.mouse.move(u * u * x0 + 2 * u * e * cx + e * e * x, u * u * y0 + 2 * u * e * cy + e * e * y);
    if (t < 1) await pause(page, 6);
  }
  at = { x, y };
}

/** Move like a person: a short eased glide, then click. */
async function clickOn(page, locator) {
  const el = locator.first();
  await el.scrollIntoViewIfNeeded();
  const box = await el.boundingBox();
  await glide(page, box.x + box.width / 2, box.y + box.height / 2);
  await pause(page, 140);
  await page.mouse.down();
  await pause(page, 70);
  await page.mouse.up();
}

async function record(browser, scheme, tmp) {
  const ctx = await browser.newContext({
    viewport: SIZE,
    colorScheme: scheme,
    locale: 'en-GB',
    timezoneId: 'UTC',
    recordVideo: { dir: tmp, size: SIZE },
  });
  await ctx.addInitScript(cursor);
  const page = await ctx.newPage();
  await page.clock.setFixedTime(NOW);
  at = { x: 980, y: 120 };
  await page.mouse.move(980, 120);

  await page.goto(`${ORIGIN}/dashboard/`);
  await page.getByRole('heading', { name: 'Upcoming' }).first().waitFor();
  await pause(page, 1300);

  await clickOn(page, page.getByRole('link', { name: 'Compose', exact: true }));
  await page.getByRole('group', { name: 'Publish to' }).waitFor();
  await pause(page, 500);
  await clickOn(page, page.getByLabel('Title (optional)'));
  await page.keyboard.type('Release 2.5 teaser', { delay: 38 });
  await clickOn(page, page.getByRole('button', { name: /Jordan Lee/ }));
  await clickOn(page, page.getByRole('button', { name: /Studio Updates/ }));
  await clickOn(page, page.getByLabel('Post content'));
  await page.keyboard.type('Release 2.5 lands next week. Scheduling that understands your timezone, finally.', { delay: 24 });
  await pause(page, 500);
  await page.locator('#sched-date').fill('2026-10-15');
  await page.locator('#sched-time').fill('09:00');
  await page.locator('#sched-time').blur();
  await pause(page, 700);
  const poster = await page.screenshot({ type: 'jpeg', quality: 88 });
  await clickOn(page, page.getByRole('button', { name: 'Schedule', exact: true }));
  await page.waitForURL(/\/posts\//);
  await page.getByText('Scheduled', { exact: true }).first().waitFor();
  await pause(page, 1500);

  await clickOn(page, page.getByRole('link', { name: 'Approvals' }));
  await page.getByRole('button', { name: /^Approve/ }).first().waitFor();
  await pause(page, 1300);
  await clickOn(page, page.getByRole('button', { name: /^Approve/ }).first());
  await pause(page, 1800);

  const video = page.video();
  await ctx.close();
  return { video: await video.path(), poster };
}

function ffmpeg(args) {
  const r = spawnSync('ffmpeg', ['-y', '-loglevel', 'error', ...args], { encoding: 'utf8' });
  if (r.status !== 0) throw new Error(`ffmpeg ${args.join(' ')}\n${r.stderr}`);
}

/** Trim the blank first frames, loop-friendly: ends on the approved state, starts on the dashboard. */
function transcode(src, poster, name) {
  const base = join(out, name);
  const common = ['-i', src, '-ss', '0.6', '-an', '-vf', 'fps=25,scale=1280:-2:flags=lanczos'];
  ffmpeg([...common, '-c:v', 'libvpx-vp9', '-b:v', '0', '-crf', '43', '-row-mt', '1', '-pix_fmt', 'yuv420p', `${base}.webm`]);
  ffmpeg([...common, '-c:v', 'libx264', '-preset', 'slow', '-crf', '29', '-pix_fmt', 'yuv420p', '-movflags', '+faststart', `${base}.mp4`]);
  writeFileSync(`${base}.jpg`, poster);
}

mkdirSync(out, { recursive: true });
const tmp = mkdtempSync(join(tmpdir(), 'hero-rec-'));
const server = await startServer({ dir: demoDir, port: PORT, base: '/steerpost/demo/' });
const browser = await launch();
try {
  for (const scheme of ['light', 'dark']) {
    const { video, poster } = await record(browser, scheme, tmp);
    transcode(video, poster, `hero-${scheme}`);
    console.log(`hero-${scheme}.{webm,mp4,jpg}`);
  }
} finally {
  await browser.close();
  server.close();
  rmSync(tmp, { recursive: true, force: true });
}
