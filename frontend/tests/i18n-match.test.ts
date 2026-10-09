import { describe, expect, it } from 'vitest';
import { LOCALES } from '@/i18n/locales';
import { matchLocale } from '@/i18n/match';
import { resolveLocale } from '@/i18n/resolve';

const ALL = LOCALES;

describe('matchLocale (every locale enabled)', () => {
  const cases: [string[], string][] = [
    [['uk'], 'en'], // never ru
    [['uk-UA', 'ru'], 'en'],
    [['pt-PT'], 'pt-BR'],
    [['pt'], 'pt-BR'],
    [['zh-TW'], 'en'],
    [['zh-HK'], 'en'],
    [['zh-Hant'], 'en'],
    [['zh-Hant-CN'], 'en'],
    [['zh-Hans-SG'], 'zh-CN'],
    [['zh-SG'], 'zh-CN'],
    [['zh-CN'], 'zh-CN'],
    [['zh'], 'zh-CN'],
    [['in'], 'id'],
    [['id-ID'], 'id'],
    [['es-MX'], 'es'],
    [['de-AT'], 'de'],
    [['de_CH'], 'de'],
    [['fr-CA'], 'fr'],
    [['ru-RU'], 'ru'],
    [['ja-JP'], 'ja'],
    [['kk-KZ'], 'kk'],
    [['ar-EG'], 'ar'],
    [['en-US'], 'en'],
    [['tr', 'de'], 'de'], // unknown tags are skipped
    [['xx'], 'en'],
    [[], 'en'],
  ];
  it.each(cases)('%j -> %s', (tags, want) => {
    expect(matchLocale(tags, ALL)).toBe(want);
  });
});

describe('matchLocale with a partial rollout', () => {
  it('skips a recognised locale that is not enabled yet', () => {
    expect(matchLocale(['ru', 'es'], ['en', 'es'])).toBe('es');
    expect(matchLocale(['ru'], ['en'])).toBe('en');
  });
  it('an explicit English tag wins over later tags', () => {
    expect(matchLocale(['en-GB', 'de'], ['en', 'de'])).toBe('en');
  });
});

describe('resolveLocale order: user -> localStorage -> navigator -> en', () => {
  const opts = { enabled: ['en', 'de', 'es'] as const, pseudo: false };
  it('user setting wins', () => {
    expect(resolveLocale({ user: 'de', stored: 'es', languages: ['es'] }, opts)).toBe('de');
  });
  it('then localStorage', () => {
    expect(resolveLocale({ user: null, stored: 'es', languages: ['de'] }, opts)).toBe('es');
  });
  it('then navigator.languages', () => {
    expect(resolveLocale({ stored: null, languages: ['de-AT'] }, opts)).toBe('de');
  });
  it('then en', () => {
    expect(resolveLocale({ languages: ['ja'] }, opts)).toBe('en');
  });
  it('ignores a stored value that is not enabled or not a locale', () => {
    expect(resolveLocale({ stored: 'ru', languages: ['es'] }, opts)).toBe('es');
    expect(resolveLocale({ stored: 'klingon', languages: [] }, opts)).toBe('en');
  });
  it('the pseudo-locale is accepted only when pseudo is on', () => {
    expect(resolveLocale({ stored: 'en-XA' }, { ...opts, pseudo: true })).toBe('en-XA');
    expect(resolveLocale({ stored: 'en-XA' }, opts)).toBe('en');
  });
});
