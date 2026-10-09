import { readFileSync } from 'node:fs';
import path from 'node:path';
import { describe, expect, it } from 'vitest';
import config from '../tailwind.config';
import { BRAND_HEX } from '../src/lib/brand';

const tokensPath = path.resolve(__dirname, '../src/styles/tokens.css');
const css = readFileSync(tokensPath, 'utf8');

function block(selector: string): Record<string, string> {
  const m = css.match(new RegExp(`^${selector.replace('.', '\\.')} \\{([\\s\\S]*?)^\\}`, 'm'));
  if (!m) throw new Error(`no ${selector} block in tokens.css`);
  const out: Record<string, string> = {};
  for (const d of (m[1] ?? '').matchAll(/^\s*--([\w-]+):\s*([^;]+);/gm)) out[d[1] ?? ''] = (d[2] ?? '').trim();
  return out;
}
const light = block(':root');
const dark = block('.dark');

function luminance(channels: string): number {
  const [h = 0, s = 0, l = 0] = channels.split(/\s+/).map((v) => parseFloat(v));
  const a = (s / 100) * Math.min(l / 100, 1 - l / 100);
  const f = (n: number) => {
    const k = (n + h / 30) % 12;
    const c = l / 100 - a * Math.max(-1, Math.min(k - 3, Math.min(9 - k, 1)));
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  };
  return 0.2126 * f(0) + 0.7152 * f(8) + 0.0722 * f(4);
}
function contrast(a: string, b: string): number {
  const hi = Math.max(luminance(a), luminance(b));
  const lo = Math.min(luminance(a), luminance(b));
  return (hi + 0.05) / (lo + 0.05);
}

describe('design tokens', () => {
  it('defines every dark colour in the light theme too', () => {
    const colours = Object.keys(dark).filter((k) => !k.startsWith('shadow'));
    for (const k of colours) expect(light, k).toHaveProperty(k);
  });

  // Pairs the UI relies on for body text. Brand pairs (accent, success, warning on the page) are included so the palette cannot drift below AA.
  const pairs: [string, string][] = [
    ['foreground', 'background'],
    ['foreground', 'surface'],
    ['muted-foreground', 'background'],
    ['muted-foreground', 'muted'],
    ['accent-foreground', 'accent'],
    ['accent', 'accent-soft'],
    ['accent', 'background'],
    ['accent', 'surface'],
    ['muted-foreground', 'surface'],
    ['success', 'background'],
    ['warning', 'background'],
    ['success', 'success-soft'],
    ['warning', 'warning-soft'],
    ['danger', 'danger-soft'],
    ['danger', 'background'],
    ['info', 'background'],
    ['info', 'info-soft'],
    ['secondary-foreground', 'secondary'],
    ['muted-foreground', 'secondary'],
    ['foreground', 'canvas'],
    ['muted-foreground', 'canvas'],
    ['accent', 'canvas'],
  ];
  for (const [name, theme] of [['light', light], ['dark', { ...light, ...dark }]] as const) {
    it.each(pairs)(`%s on %s meets WCAG AA (4.5:1) in the ${name} theme`, (fg, bg) => {
      expect(contrast(theme[fg] ?? '', theme[bg] ?? '')).toBeGreaterThanOrEqual(4.5);
    });
  }

  // Control outlines are non-text UI: WCAG 1.4.11 asks for 3:1 against what they sit on.
  for (const [name, theme] of [['light', light], ['dark', { ...light, ...dark }]] as const) {
    it.each([['input', 'background'], ['input', 'surface'], ['input', 'canvas'], ['ring', 'canvas']])(`%s on %s meets WCAG 1.4.11 (3:1) in the ${name} theme`, (fg, bg) => {
      expect(contrast(theme[fg] ?? '', theme[bg] ?? '')).toBeGreaterThanOrEqual(3);
    });
  }

  it('gives every theme the same glass, mesh and motion tokens', () => {
    for (const k of ['glass-blur-chrome', 'glass-blur-strong', 'glass-blur-hero', 'glass-saturate', 'duration-hero', 'ease-fill', 'font-script']) {
      expect(light, k).toHaveProperty(k);
    }
  });

  it('only references tokens that exist', () => {
    const source = JSON.stringify(config);
    const used = [...source.matchAll(/var\(--([\w-]+)\)/g)].map((m) => m[1] ?? '');
    expect(used.length).toBeGreaterThan(20);
    for (const name of used) expect(light, name).toHaveProperty(name);
  });
});

type Rgb = [number, number, number];

/** sRGB 0..1 from an `h s% l%` triple. */
function rgbOf(channels: string): Rgb {
  const [h = 0, s = 0, l = 0] = channels.split(/\s+/).map((v) => parseFloat(v));
  const a = (s / 100) * Math.min(l / 100, 1 - l / 100);
  const f = (n: number) => {
    const k = (n + h / 30) % 12;
    return l / 100 - a * Math.max(-1, Math.min(k - 3, Math.min(9 - k, 1)));
  };
  return [f(0), f(8), f(4)];
}
/** A complete `hsl(h s% l% / a)` token as colour plus alpha. */
function hslaOf(value: string): { rgb: Rgb; alpha: number } {
  const m = value.match(/^hsl\(([^/]+)\/\s*([\d.]+)\)$/);
  if (!m) throw new Error(`not an hsl(... / a) colour: ${value}`);
  return { rgb: rgbOf((m[1] ?? '').trim()), alpha: parseFloat(m[2] ?? '1') };
}
function over(top: { rgb: Rgb; alpha: number }, base: Rgb): Rgb {
  return top.rgb.map((c, i) => c * top.alpha + (base[i] ?? 0) * (1 - top.alpha)) as Rgb;
}
function relLum([r, g, b]: Rgb): number {
  const f = (c: number) => (c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4);
  return 0.2126 * f(r) + 0.7152 * f(g) + 0.0722 * f(b);
}
function ratio(a: Rgb, b: Rgb): number {
  const hi = Math.max(relLum(a), relLum(b));
  const lo = Math.min(relLum(a), relLum(b));
  return (hi + 0.05) / (lo + 0.05);
}

// Text on glass (D-024) is checked against the worst case the eye can meet: the glass tint composited over the canvas and
// over each mesh colour at full strength (the centre of a blob). Inputs never sit on glass, so only text pairs are here.
describe('text on glass', () => {
  const texts = ['foreground', 'muted-foreground', 'accent', 'secondary-foreground', 'success', 'warning', 'danger', 'info'];
  for (const [name, theme] of [['light', light], ['dark', { ...light, ...dark }]] as const) {
    const canvas = rgbOf(theme.canvas ?? '');
    const backdrops = [canvas, ...['mesh-1', 'mesh-2', 'mesh-3'].map((k) => over(hslaOf(theme[k] ?? ''), canvas))];
    for (const glass of ['glass-chrome', 'glass-card', 'glass-strong']) {
      it.each(texts)(`%s on ${glass} meets AA over the mesh in the ${name} theme`, (fg) => {
        const worst = Math.min(...backdrops.map((b) => ratio(rgbOf(theme[fg] ?? ''), over(hslaOf(theme[glass] ?? ''), b))));
        expect(worst).toBeGreaterThanOrEqual(4.5);
      });
    }
  }
});

function toHex(channels: string): string {
  const [h = 0, s = 0, l = 0] = channels.split(/\s+/).map((v) => parseFloat(v));
  const a = (s / 100) * Math.min(l / 100, 1 - l / 100);
  const f = (n: number) => {
    const k = (n + h / 30) % 12;
    const c = l / 100 - a * Math.max(-1, Math.min(k - 3, Math.min(9 - k, 1)));
    return Math.round(255 * c).toString(16).padStart(2, '0');
  };
  return `#${f(0)}${f(8)}${f(4)}`;
}

describe('brand hex values', () => {
  it('match the HSL tokens they mirror (manifest, theme-color)', () => {
    const darkTheme = { ...light, ...dark };
    expect(BRAND_HEX.light).toEqual({ background: toHex(light.background ?? ''), accent: toHex(light.accent ?? '') });
    expect(BRAND_HEX.dark).toEqual({ background: toHex(darkTheme.background ?? ''), accent: toHex(darkTheme.accent ?? '') });
  });
});

describe('cn with custom tokens', () => {
  it('lets custom font sizes, shadows and z-indexes replace the base class', async () => {
    const { cn } = await import('../src/lib/utils');
    expect(cn('text-sm', 'text-compact')).toBe('text-compact');
    expect(cn('text-sm', 'text-2xs')).toBe('text-2xs');
    expect(cn('shadow-md', 'shadow-pop')).toBe('shadow-pop');
    expect(cn('z-overlay', 'z-toast')).toBe('z-toast');
    expect(cn('text-sm', 'text-muted-foreground')).toBe('text-sm text-muted-foreground');
  });
});
