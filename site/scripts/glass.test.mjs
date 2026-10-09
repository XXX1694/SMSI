// The landing's glass (D-024, BRAND section 4): the tokens are the app's, the fallbacks exist, and text stays AA on the
// mesh and on every glass layer. tokens.css is a copy of frontend/src/styles/tokens.css, so this reads the source.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';

const root = join(dirname(fileURLToPath(import.meta.url)), '../..');
const read = (f) => readFileSync(join(root, f), 'utf8');
const tokens = read('frontend/src/styles/tokens.css');
const landing = read('site/src/assets/landing.css');
const heroCss = read('site/src/assets/hero.css');
const hero = heroCss + read('site/src/assets/route.css');
// hero.css: --hero-glow is the strongest accent layer behind the headline (light, then the dark override).
const glowAlphas = [...heroCss.matchAll(/--hero-glow: hsl\(var\(--accent\) \/ ([\d.]+)\)/g)].map((m) => parseFloat(m[1]));
assert.equal(glowAlphas.length, 2, 'hero.css must define --hero-glow for light and dark');

function block(selector) {
  const m = tokens.match(new RegExp(`^${selector.replace('.', '\\.')} \\{([\\s\\S]*?)^\\}`, 'm'));
  assert.ok(m, `no ${selector} block in tokens.css`);
  return Object.fromEntries([...m[1].matchAll(/^\s*--([\w-]+):\s*([^;]+);/gm)].map((d) => [d[1], d[2].trim()]));
}
const light = block(':root');
const themes = { light, dark: { ...light, ...block('.dark') } };

const rgbOf = (channels) => {
  const [h = 0, s = 0, l = 0] = channels.split(/\s+/).map(parseFloat);
  const a = (s / 100) * Math.min(l / 100, 1 - l / 100);
  const f = (n) => { const k = (n + h / 30) % 12; return l / 100 - a * Math.max(-1, Math.min(k - 3, Math.min(9 - k, 1))); };
  return [f(0), f(8), f(4)];
};
const hslaOf = (v) => { const m = v.match(/^hsl\(([^/]+)\/\s*([\d.]+)\)$/); assert.ok(m, `not hsl(... / a): ${v}`); return { rgb: rgbOf(m[1].trim()), alpha: parseFloat(m[2]) }; };
const over = (top, base) => top.rgb.map((c, i) => c * top.alpha + base[i] * (1 - top.alpha));
const lum = ([r, g, b]) => { const f = (c) => (c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4); return 0.2126 * f(r) + 0.7152 * f(g) + 0.0722 * f(b); };
const ratio = (a, b) => (Math.max(lum(a), lum(b)) + 0.05) / (Math.min(lum(a), lum(b)) + 0.05);

const strength = parseFloat(landing.match(/body\.landing \{ --mesh-strength: ([\d.]+)/)?.[1] ?? 'NaN');
const texts = ['foreground', 'muted-foreground', 'accent'];

test('the landing sets its own mesh strength, and it is not stronger than the mesh colours allow', () => {
  assert.ok(strength > 0 && strength <= 1, `mesh strength ${strength}`);
});

for (const [name, t] of Object.entries(themes)) {
  const canvas = rgbOf(t.canvas);
  const mesh = ['mesh-1', 'mesh-2', 'mesh-3'].map((k) => { const m = hslaOf(t[k]); return over({ rgb: m.rgb, alpha: m.alpha * strength }, canvas); });
  // The hero also carries the accent glow on top of the strongest mesh point.
  const glowAlpha = glowAlphas[name === 'light' ? 0 : 1];
  const glow = (b) => over({ rgb: rgbOf(t.accent), alpha: glowAlpha }, b);
  const plain = [canvas, ...mesh];
  const withGlow = plain.map(glow);

  for (const fg of texts) {
    test(`${fg} meets AA on the plain mesh (${name})`, () => {
      assert.ok(Math.min(...plain.map((b) => ratio(rgbOf(t[fg]), b))) >= 4.5);
    });
    // Muted text (the hero lead) sits on the glow; accent text is a link or eyebrow outside the hero glow.
    if (fg !== 'accent') test(`${fg} meets AA on the mesh under the hero glow (${name})`, () => {
      const w = Math.min(...withGlow.map((b) => ratio(rgbOf(t[fg]), b))); assert.ok(w >= 4.5, `${w}`);
    });
    for (const glass of ['glass-chrome', 'glass-card']) {
      test(`${fg} meets AA on ${glass} over the strongest mesh and glow (${name})`, () => {
        const g = hslaOf(t[glass]);
        assert.ok(Math.min(...[...plain, ...withGlow].map((b) => ratio(rgbOf(t[fg]), over(g, b)))) >= 4.5);
      });
    }
  }
}

test('glass never goes below 0.66 alpha under text', () => {
  for (const t of Object.values(themes)) for (const k of ['glass-chrome', 'glass-card', 'glass-strong']) assert.ok(hslaOf(t[k]).alpha >= 0.66, k);
});

test('the floating nav is near-opaque so text scrolling under it never reads through', () => {
  assert.match(landing, /:where\(\.lp-nav-bar\)\.glass-chrome \{ background-color: hsl\(var\(--background\) \/ 0\.9\); \}/);
});

test('blur radii come from the shared tokens and the budget is the nav plus the hero frame', () => {
  assert.equal(light['glass-blur-chrome'], '20px');
  assert.equal(light['glass-blur-hero'], '16px');
  assert.match(landing, /\.glass-chrome \{[^}]*blur\(var\(--glass-blur-chrome\)\)/);
  assert.match(landing, /\.glass-hero \{[^}]*blur\(var\(--glass-blur-hero\)\)/);
  assert.doesNotMatch((landing + hero).replace(/blur\(1px\)/g, ''), /blur\(\d/, 'a hard-coded blur radius forks the tokens');
  assert.equal([...landing.matchAll(/(?<!-webkit-)backdrop-filter: blur\(var/g)].length, 2, 'only chrome and the hero frame blur');
});

test('fallbacks: no backdrop-filter, reduced transparency and forced colours', () => {
  assert.match(landing, /@supports not \(\(backdrop-filter: blur\(1px\)\) or \(-webkit-backdrop-filter: blur\(1px\)\)\)/);
  assert.match(landing, /@media \(prefers-reduced-transparency: reduce\)[^@]*body\.landing::before, body\.landing::after \{ display: none/);
  assert.match(landing, /@media \(forced-colors: active\)[^@]*border-color: CanvasText[^@]*backdrop-filter: none/);
});

test('docs pages and long text stay solid: style.css and layout.html carry no glass', () => {
  assert.doesNotMatch(read('site/src/assets/style.css') + read('site/src/layout.html'), /glass-|--mesh/);
});

test('the nav bar blurs: no ancestor of it is a backdrop root (a view-transition-name on the header blinds the blur)', () => {
  assert.match(landing, /\.lp-nav \{[^}]*view-transition-name: none/);
  assert.match(landing, /\.lp-nav-bar \{[^}]*view-transition-name: site-nav/);
});
