import { readFileSync } from 'node:fs';
import path from 'node:path';
import { describe, expect, it } from 'vitest';
import config from '../tailwind.config';

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

  // Pairs the UI relies on for body text. Values are unchanged from before tokens moved to one file.
  const pairs: [string, string][] = [
    ['foreground', 'background'],
    ['foreground', 'surface'],
    ['muted-foreground', 'background'],
    ['muted-foreground', 'muted'],
    ['accent-foreground', 'accent'],
    ['accent', 'accent-soft'],
    ['success', 'success-soft'],
    ['warning', 'warning-soft'],
    ['danger', 'danger-soft'],
    ['danger', 'background'],
  ];
  for (const [name, theme] of [['light', light], ['dark', { ...light, ...dark }]] as const) {
    it.each(pairs)(`%s on %s meets WCAG AA (4.5:1) in the ${name} theme`, (fg, bg) => {
      expect(contrast(theme[fg] ?? '', theme[bg] ?? '')).toBeGreaterThanOrEqual(4.5);
    });
  }

  it('only references tokens that exist', () => {
    const source = JSON.stringify(config);
    const used = [...source.matchAll(/var\(--([\w-]+)\)/g)].map((m) => m[1] ?? '');
    expect(used.length).toBeGreaterThan(20);
    for (const name of used) expect(light, name).toHaveProperty(name);
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
