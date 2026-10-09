import { beforeEach, describe, expect, it } from 'vitest';
import { localeScript } from '@/i18n/head-script';

function run(stored: string | null, enabled: string[]) {
  window.localStorage.clear();
  if (stored) window.localStorage.setItem('steerpost_locale', stored);
  const h = document.documentElement;
  h.lang = 'en';
  h.dir = 'ltr';
  h.removeAttribute('data-i18n-pending');
  new Function(localeScript(enabled))();
  return h;
}

describe('head script', () => {
  beforeEach(() => window.localStorage.clear());
  it('applies an enabled stored locale, rtl for ar, and hides the shell until the catalog is ready', () => {
    const h = run('ar', ['en', 'ar']);
    expect([h.lang, h.dir, h.hasAttribute('data-i18n-pending')]).toEqual(['ar', 'rtl', true]);
  });
  it('ignores a locale that is not enabled', () => {
    const h = run('ru', ['en']);
    expect([h.lang, h.dir, h.hasAttribute('data-i18n-pending')]).toEqual(['en', 'ltr', false]);
  });
  it('does not hide anything when the server already rendered that locale', () => {
    window.localStorage.setItem('steerpost_locale', 'ar');
    document.documentElement.lang = 'ar';
    new Function(localeScript(['en', 'ar']))();
    expect(document.documentElement.hasAttribute('data-i18n-pending')).toBe(false);
  });
});
