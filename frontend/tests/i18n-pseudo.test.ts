import { describe, expect, it } from 'vitest';
import { formatIcu } from '@/i18n/icu';
import { pseudoCatalog, pseudoMessage, pseudoText } from '@/i18n/pseudo';
import en from '../messages/en.json';

describe('pseudo-locale', () => {
  it('accents and expands about 35 %', () => {
    const out = pseudoText('Dashboard');
    expect(out).not.toBe('Dashboard');
    expect(out.length).toBeGreaterThanOrEqual(Math.ceil('Dashboard'.length * 1.35));
    expect(out).toMatch(/^Ď/);
  });
  it('wraps in brackets and leaves ICU syntax working', () => {
    const msg = '{count, plural, one {# attempt} other {# attempts}} for <b>{name}</b>';
    const out = pseudoMessage(msg);
    expect(out.startsWith('[')).toBe(true);
    expect(out.endsWith(']')).toBe(true);
    const text = formatIcu(out, 'en', { count: 3, name: 'Ana' });
    expect(text).toContain('Ana');
    expect(text).toContain('3');
    expect(text).not.toMatch(/\battempts\b/); // translated to accented text
  });
  it('keeps text without letters and unparsable input intact', () => {
    expect(pseudoText('123 · ✓')).toBe('123 · ✓');
    expect(pseudoMessage('{broken')).toBe('[{broken]');
  });
  it('turns every English string into a different string with the same shape', () => {
    const out = pseudoCatalog(en) as typeof en;
    expect(Object.keys(out)).toEqual(Object.keys(en));
    expect(out.nav.dashboard).not.toBe(en.nav.dashboard);
    expect(out.language.optionBeta).toContain('{name}');
  });
});
