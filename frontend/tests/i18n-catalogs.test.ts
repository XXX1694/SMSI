import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';
import { ENABLED_LOCALES, LOCALES } from '@/i18n/locales';
import { createTranslator } from '@/i18n/translate';
import { flatten, listSources, literals, problems, usedKeys } from '../scripts/i18n-lib.mjs';
import en from '../messages/en.json';
import meta from '../messages/meta.json';

const root = join(__dirname, '..');
const catalogs: Record<string, object> = {};
for (const l of LOCALES) catalogs[l] = JSON.parse(readFileSync(join(root, 'messages', `${l}.json`), 'utf8'));

describe('real catalogs', () => {
  it('every catalog is valid and a subset of English; enabled ones are complete', () => {
    const { errors } = problems({
      catalogs,
      enabled: [...ENABLED_LOCALES],
      meta,
      sources: listSources(join(root, 'src')).map((p) => readFileSync(p, 'utf8')),
    });
    expect(errors).toEqual([]);
  });

  it('there is a catalog file for every locale and none for an unknown one', () => {
    const files = readdirSync(join(root, 'messages')).filter((f) => f !== 'meta.json').map((f) => f.replace('.json', ''));
    expect(files.sort()).toEqual([...LOCALES].sort());
  });

  // Scaffold for the extraction PRs: as soon as code calls t('...') with a literal, English must define it.
  it('English has every key that code asks for with a literal', () => {
    const flat = flatten(en);
    const used = new Set<string>();
    for (const file of listSources(join(root, 'src'))) for (const k of usedKeys(readFileSync(file, 'utf8'))) used.add(k);
    expect(used.size).toBeGreaterThan(0); // Settings uses tl('label') etc.
    expect([...used].filter((k) => !(k in flat))).toEqual([]);
  });

  it('translated catalogs reference English keys only', () => {
    const flat = flatten(en);
    for (const [locale, cat] of Object.entries(catalogs)) {
      for (const k of Object.keys(flatten(cat))) expect(k in flat, `${locale}: ${k}`).toBe(true);
    }
  });
});

describe('problems() catches what CI must catch', () => {
  const base = { nav: { a: 'A', b: '{n, plural, one {# x} other {# xs}}' } };
  const run = (ru: object, enabled = ['en', 'ru']) => problems({ catalogs: { en: base, ru }, enabled, meta: {}, sources: [] });

  it('an extra key', () => expect(run({ nav: { zzz: 'Z' } }, ['en']).errors.join()).toContain('not an English key'));
  it('a missing key in an enabled locale', () => expect(run({ nav: { a: 'А' } }).errors.join()).toContain('missing key nav.b'));
  it('a missing key is fine in a locale that is not enabled', () => expect(run({ nav: { a: 'А' } }, ['en']).errors).toEqual([]));
  it('invalid ICU', () => expect(run({ nav: { a: '{broken' } }, ['en']).errors.join()).toContain('not valid ICU'));
  it('placeholders must match English', () => expect(run({ nav: { b: '{m, plural, one {# x} other {# xs}}' } }, ['en']).errors.join()).toContain('placeholders differ'));
  it('Russian plurals need few and many', () => {
    const errors = run({ nav: { a: 'А', b: '{n, plural, one {# x} other {# xs}}' } }).errors.join();
    expect(errors).toContain('lacks plural categories');
  });
  it('code using a key English lacks', () => {
    const src = "const t = useTranslations('nav'); t('a'); t('nope');";
    expect(problems({ catalogs: { en: base }, enabled: ['en'], sources: [src] }).errors.join()).toContain('nav.nope');
  });
  it('an unused key is only a warning', () => {
    const r = problems({ catalogs: { en: base }, enabled: ['en'], sources: ["const t = useTranslations('nav'); t('a');"] });
    expect(r.errors).toEqual([]);
    expect(r.warnings.join()).toContain('unused key nav.b');
  });
});

describe('usedKeys / literals', () => {
  it('resolves namespaces per variable and ignores other functions', () => {
    const src = `
      const t = useTranslations('nav'); const x = useTranslations();
      t('dashboard'); t.rich('compose', {}); x('language.label'); other('nope');`;
    expect(usedKeys(src)).toEqual(['nav.dashboard', 'nav.compose', 'language.label']);
  });
  it('collects string literals', () => {
    expect([...literals(`a('x.y'); b("z")`)]).toEqual(['x.y', 'z']);
  });
});

describe('typed keys', () => {
  const t = createTranslator({ locale: 'en', messages: en }, 'nav');
  it('resolve and format', () => {
    expect(t('dashboard')).toBe('Dashboard');
    expect(createTranslator({ locale: 'en', messages: en })('language.optionBeta', { name: 'Русский' })).toBe('Русский (Beta translation)');
  });
  it('reject unknown keys at compile time and report them at runtime', () => {
    // @ts-expect-error not a key of messages/en.json
    expect(createTranslator({ locale: 'en', messages: en, onMissing: () => {} }, 'nav')('nonsense')).toBe('nav.nonsense');
    // @ts-expect-error not a namespace
    createTranslator({ locale: 'en', messages: en }, 'nonsense');
  });
});
