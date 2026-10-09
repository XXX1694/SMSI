import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';
import { ENABLED_LOCALES, LOCALES } from '@/i18n/locales';
import { createTranslator } from '@/i18n/translate';
import { catalogBundles, flatten, layoutProblems, listBundles, listSources, literals, problems, readLocale, usedKeys } from '../scripts/i18n-lib.mjs';
import en from '@/i18n/en-all';
import meta from '../messages/meta.json';

const root = join(__dirname, '..');
const catalogs: Record<string, object> = {};
for (const l of LOCALES) catalogs[l] = readLocale(join(root, 'messages', l));

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

  it('there is a directory for no unknown locale, and catalog.ts lists the bundles of messages/en/', () => {
    const dirs = readdirSync(join(root, 'messages'), { withFileTypes: true }).filter((f) => f.isDirectory()).map((f) => f.name);
    expect(dirs.filter((d) => !(LOCALES as readonly string[]).includes(d))).toEqual([]);
    expect(readdirSync(join(root, 'messages')).filter((f) => f.endsWith('.json'))).toEqual(['meta.json']);
    const bundles = Object.fromEntries(LOCALES.map((l) => [l, listBundles(join(root, 'messages', l))]));
    const catalog = catalogBundles(readFileSync(join(root, 'src/i18n/catalog.ts'), 'utf8'));
    expect(layoutProblems({ bundles, catalog })).toEqual([]);
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

describe('English fallback', () => {
  const messages = { nav: { a: '{broken', b: 'B-local' } } as never;
  const t = createTranslator({ locale: 'ru', messages, fallback: { nav: { a: 'A-en', b: 'B-en', c: 'C-en' } }, onMissing: () => {} });
  it('a broken message falls back to English, then to the key', () => {
    expect(t('nav.a' as never)).toBe('A-en');
    expect(t('nav.b' as never)).toBe('B-local');
    expect(t('nav.c' as never)).toBe('C-en');
    expect(t('nav.zzz' as never)).toBe('nav.zzz');
  });
});

describe('bundle layout', () => {
  const catalog = { list: ['nav', 'posts'], typed: ['nav', 'posts'] };

  it('accepts a locale with some of the bundles, or none', () => {
    expect(layoutProblems({ bundles: { en: ['nav', 'posts'], ru: ['nav'], ar: [] }, catalog })).toEqual([]);
  });

  it('rejects a bundle file that English lacks', () => {
    expect(layoutProblems({ bundles: { en: ['nav', 'posts'], ru: ['nav', 'extra'] }, catalog })).toEqual(['messages/ru/extra.json: English has no extra.json']);
  });

  it('rejects a catalog.ts list that differs from messages/en/ in either direction', () => {
    const errors = layoutProblems({ bundles: { en: ['nav', 'posts', 'media'] }, catalog: { list: ['nav', 'posts', 'gone'], typed: ['nav', 'posts', 'media'] } });
    expect(errors).toEqual([
      'src/i18n/catalog.ts: BUNDLES lists gone, but messages/en/gone.json does not exist',
      'messages/en/media.json is not in BUNDLES (src/i18n/catalog.ts)',
    ]);
  });

  it('rejects a Messages type that differs from messages/en/', () => {
    const errors = layoutProblems({ bundles: { en: ['nav', 'posts'] }, catalog: { list: ['nav', 'posts'], typed: ['nav'] } });
    expect(errors).toEqual(['messages/en/posts.json is not in the Messages type (src/i18n/catalog.ts)']);
  });

  it('reads both lists out of catalog.ts source', () => {
    const src = "export const BUNDLES = [\n  'nav',\n  'posts',\n] as const;\nexport type Messages = {\n  nav: typeof import('../../messages/en/nav.json');\n};";
    expect(catalogBundles(src)).toEqual({ list: ['nav', 'posts'], typed: ['nav'] });
  });
});
