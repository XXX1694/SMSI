#!/usr/bin/env node
/**
 * Generates the Steerpost brand files from one place, so the mark, the colours and every raster stay in step:
 *
 *   node scripts/build-brand.mjs        (PNGs need CHROMIUM_PATH, e.g. Google Chrome on macOS)
 *
 * Colours are read from frontend/src/styles/tokens.css (the single source), never typed twice. The wordmark and the
 * tagline are Inter outlines (brand-outlines.json, made from the vendored OFL font with fontTools), so the SVGs look the
 * same on GitHub, in an <img> and in a browser without the font. Rules and the reasoning live in docs/BRAND.md.
 */
import { mkdirSync, readFileSync, writeFileSync, copyFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const root = resolve(here, '../..');
const out = (p) => { const f = join(root, p); mkdirSync(dirname(f), { recursive: true }); return f; };
const put = (p, s) => writeFileSync(out(p), s.endsWith('\n') ? s : `${s}\n`);

// ---- colours ---------------------------------------------------------------------------------------------------
const css = readFileSync(join(root, 'frontend/src/styles/tokens.css'), 'utf8');
const grab = (re) => Object.fromEntries([...css.match(re)[1].matchAll(/--([\w-]+):\s*(\d+(?:\.\d+)?) (\d+(?:\.\d+)?)% (\d+(?:\.\d+)?)%;/g)].map((m) => [m[1], [+m[2], +m[3], +m[4]]]));
const lightT = grab(/^:root \{([\s\S]*?)^\}/m);
const darkT = { ...lightT, ...grab(/^\.dark \{([\s\S]*?)^\}/m) };
const hex = ([h, s, l]) => {
  s /= 100; l /= 100;
  const a = s * Math.min(l, 1 - l);
  const f = (n) => { const k = (n + h / 30) % 12; return Math.round(255 * (l - a * Math.max(-1, Math.min(k - 3, Math.min(9 - k, 1))))).toString(16).padStart(2, '0'); };
  return `#${f(0)}${f(8)}${f(4)}`;
};
const C = (t) => ({ bg: hex(t.background), surface: hex(t.surface), fg: hex(t.foreground), muted: hex(t['muted-foreground']), border: hex(t.border), accent: hex(t.accent), accentFg: hex(t['accent-foreground']), accentSoft: hex(t['accent-soft']) });
const L = C(lightT);
const D = C(darkT);

// ---- the mark ---------------------------------------------------------------------------------------------------
// A lane change: the path leaves flat, bends up in two equal quarter turns and lands in a paper-plane tip.
// 64 x 64 grid, optically centred. Stroke 6.5, radius 12, tip 17 wide.
const markBody = (fill) =>
  `<path d="M7 53.5H19A12 12 0 0 0 31 41.5V33.5A12 12 0 0 1 43 21.5H46" fill="none" stroke="${fill}" stroke-width="6.5" stroke-linecap="round"/>` +
  `<path d="M42 8.5 59 21.5 42 34.5 46 21.5Z" fill="${fill}" stroke="${fill}" stroke-width="2.5" stroke-linejoin="round"/>`;
const svg = (vb, inner, label, extra = '') =>
  `<svg xmlns="http://www.w3.org/2000/svg" viewBox="${vb}"${extra} role="img" aria-label="${label}"><title>${label}</title>${inner}</svg>`;
const NAME = 'Steerpost';

// ---- wordmark and lockup ----------------------------------------------------------------------------------------
const O = JSON.parse(readFileSync(join(here, 'brand-outlines.json'), 'utf8'));
const wmScale = 24 / O.wordmark.cap; // cap height 24 in lockup units
const wordmarkOnly = (fill) => svg(`0 -1540 ${Math.round(O.wordmark.w)} 2020`, `<path d="${O.wordmark.d}" fill="${fill}"/>`, NAME);
const lockup = (markFill, wordFill) =>
  svg('0 0 202 40', `<g transform="translate(-2 -2.4) scale(.703)">${markBody(markFill)}</g><g transform="translate(52 32) scale(${wmScale.toFixed(5)})"><path d="${O.wordmark.d}" fill="${wordFill}"/></g>`, NAME);
// scale .703: 64-grid mark drawn about 45 units wide inside a 40 unit tall lockup.

const tile = (bg, fg, r = 14) =>
  `<rect width="64" height="64" rx="${r}" fill="${bg}"/><g transform="translate(32 32) scale(.66) translate(-32 -32)">${markBody(fg)}</g>`;

const BRAND_FILES = {
  'mark.svg': svg('0 0 64 64', markBody('currentColor'), NAME),
  'mark-light.svg': svg('0 0 64 64', markBody(L.accent), NAME),
  'mark-dark.svg': svg('0 0 64 64', markBody(D.accent), NAME),
  'wordmark.svg': wordmarkOnly('currentColor'),
  'wordmark-light.svg': wordmarkOnly(L.fg),
  'wordmark-dark.svg': wordmarkOnly(D.fg),
  'lockup.svg': lockup('currentColor', 'currentColor'),
  'lockup-light.svg': lockup(L.accent, L.fg),
  'lockup-dark.svg': lockup(D.accent, D.fg),
  'app-icon.svg': svg('0 0 64 64', tile(L.accent, '#ffffff'), NAME),
};
for (const dir of ['frontend/public/brand', 'site/src/assets/brand']) for (const [n, s] of Object.entries(BRAND_FILES)) put(`${dir}/${n}`, s);

const favicon = svg('0 0 64 64', tile(L.accent, '#ffffff', 15), NAME);
put('site/src/assets/favicon.svg', favicon);
put('frontend/src/app/icon.svg', favicon);
put('docs/assets/wordmark-light.svg', BRAND_FILES['lockup-light.svg']);
put('docs/assets/wordmark-dark.svg', BRAND_FILES['lockup-dark.svg']);

// ---- steering curves (banner, social preview) -------------------------------------------------------------------
// Six lanes leave an agent on the left, bend toward one approval gate, and fan out again. One lane (the sent one) is accent.
function steering({ x, y, w, h }, c, hot = 2) {
  const gx = x + w * 0.5, gy = y + h * 0.5;
  const ys0 = [0.1, 0.28, 0.44, 0.6, 0.76, 0.92], ys1 = [0.06, 0.24, 0.46, 0.58, 0.8, 0.94];
  const lane = (i) => {
    const a = y + h * ys0[i], b = y + h * ys1[i];
    return `M${x} ${a.toFixed(1)}C${(x + w * 0.27).toFixed(1)} ${a.toFixed(1)} ${(gx - w * 0.2).toFixed(1)} ${gy.toFixed(1)} ${gx.toFixed(1)} ${gy.toFixed(1)}S${(x + w * 0.73).toFixed(1)} ${b.toFixed(1)} ${x + w} ${b.toFixed(1)}`;
  };
  let s = '';
  for (let i = 0; i < 6; i++) if (i !== hot) s += `<path d="${lane(i)}" fill="none" stroke="${c.line}" stroke-width="1.5"/>`;
  s += `<path d="${lane(hot)}" fill="none" stroke="${c.accent}" stroke-width="2.5" stroke-linecap="round"/>`;
  const ey = y + h * ys1[hot];
  s += `<path d="M${x + w - 12} ${(ey - 8).toFixed(1)} ${x + w + 4} ${ey.toFixed(1)} ${x + w - 12} ${(ey + 8).toFixed(1)} ${x + w - 8} ${ey.toFixed(1)}Z" fill="${c.accent}" stroke="${c.accent}" stroke-width="2" stroke-linejoin="round"/>`;
  s += `<circle cx="${gx}" cy="${gy}" r="15" fill="${c.bg}" stroke="${c.accent}" stroke-width="2.5"/><circle cx="${gx}" cy="${gy}" r="4.5" fill="${c.accent}"/>`;
  return s;
}

function banner(t, T) {
  const W = 1280, H = 320;
  const tagScale = 30 / O.tagline.cap;
  const inner =
    `<rect width="${W}" height="${H}" fill="${t.bg}"/>` +
    steering({ x: 660, y: 36, w: 560, h: 248 }, { line: t.border, accent: t.accent, bg: t.bg }) +
    `<g transform="translate(72 82) scale(1.9)"><g transform="translate(-2 -2.4) scale(.703)">${markBody(t.accent)}</g><g transform="translate(52 32) scale(${wmScale.toFixed(5)})"><path d="${O.wordmark.d}" fill="${t.fg}"/></g></g>` +
    `<g transform="translate(72 232) scale(${tagScale.toFixed(5)})"><path d="${O.tagline.d}" fill="${t.muted}"/></g>`;
  return svg(`0 0 ${W} ${H}`, inner, `${NAME}: the human steers, the agent posts`, ` width="${W}" height="${H}"`);
}
put('docs/assets/banner-light.svg', banner(L, lightT));
put('docs/assets/banner-dark.svg', banner(D, darkT));

// ---- rasters ----------------------------------------------------------------------------------------------------
const chrome = process.env.CHROMIUM_PATH;
if (!chrome) {
  console.log('CHROMIUM_PATH not set: SVGs written, PNGs skipped.');
  process.exit(0);
}
const { chromium } = await import('playwright-core');
const browser = await chromium.launch({ executablePath: chrome });
const fontUrl = `file://${join(root, 'site/scripts/brand-fonts/inter-latin-wght-normal.woff2')}`;
async function render(html, w, h, file, transparent = false) {
  const page = await browser.newPage({ viewport: { width: w, height: h } });
  await page.setContent(`<style>@font-face{font-family:Inter;src:url(${fontUrl});font-weight:100 900}html,body{margin:0;background:transparent}</style>${html}`);
  await page.evaluate(() => document.fonts.ready);
  await page.screenshot({ path: out(file), omitBackground: transparent });
  await page.close();
}
const sized = (s, n) => s.replace('<svg ', `<svg width="${n}" height="${n}" `);
const tileSvg = (r) => svg('0 0 64 64', tile(L.accent, '#ffffff', r), NAME);
await render(sized(favicon, 32), 32, 32, 'site/src/assets/favicon-32.png', true);
// Touch icons are square: the platform rounds the corners.
await render(sized(tileSvg(0), 180), 180, 180, 'site/src/assets/apple-touch-icon.png');
await render(sized(tileSvg(0), 180), 180, 180, 'frontend/src/app/apple-icon.png');
await render(sized(favicon, 32), 32, 32, 'frontend/src/app/icon.png', true);
await render(sized(tileSvg(0), 192), 192, 192, 'frontend/public/brand/icon-192.png');
await render(sized(tileSvg(0), 512), 512, 512, 'frontend/public/brand/icon-512.png');
// Maskable: the mark stays inside the central 80% safe zone.
const maskable = svg('0 0 64 64', `<rect width="64" height="64" fill="${L.accent}"/><g transform="translate(32 32) scale(.5) translate(-32 -32)">${markBody('#ffffff')}</g>`, NAME);
await render(sized(maskable, 512), 512, 512, 'frontend/public/brand/icon-maskable-512.png');

const social = `
<div style="width:1280px;height:640px;position:relative;overflow:hidden;background:${D.bg};font-family:Inter,sans-serif;color:${D.fg}">
  <svg width="1280" height="640" viewBox="0 0 1280 640" style="position:absolute;inset:0">${steering({ x: 760, y: 90, w: 470, h: 470 }, { line: D.border, accent: D.accent, bg: D.bg })}</svg>
  <div style="position:absolute;inset:0;background:linear-gradient(90deg,${D.bg} 0,${D.bg} 50%,transparent 66%)"></div>
  <div style="position:absolute;left:84px;top:78px;width:300px">${lockup(D.accent, D.fg).replace('<svg ', '<svg width="300" ')}</div>
  <h1 style="position:absolute;left:84px;top:236px;margin:0;font-size:78px;line-height:1.02;font-weight:640;letter-spacing:-0.045em;width:640px">The human steers.<br><span style="color:${D.muted}">The agent posts.</span></h1>
  <p style="position:absolute;left:86px;bottom:78px;margin:0;font-size:25px;font-weight:450;color:${D.muted};letter-spacing:-0.01em">Self-hosted social publishing with an MCP server.</p>
</div>`;
await render(social, 1280, 640, 'docs/assets/social-preview.png');
copyFileSync(out('docs/assets/social-preview.png'), out('site/src/assets/brand/social-preview.png'));
await browser.close();
console.log('brand assets written');
