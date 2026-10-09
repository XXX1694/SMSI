import { IntlMessageFormat } from 'intl-messageformat';
import { describe, expect, it } from 'vitest';
import { formatIcu, IcuSyntaxError, parseIcu } from '@/i18n/icu';
import { formatTag } from '@/i18n/locales';

type V = Record<string, string | number>;
const corpus: [string, string, V][] = [
  ['plain', 'Hello world', {}],
  ['arg', 'Connected to {network}', { network: 'LinkedIn' }],
  ['apostrophe', "It's fine, don't worry", {}],
  ['quoted braces', "Use '{'curly'}' braces", {}],
  ['doubled apostrophe', "It''s {n}", { n: 3 }],
  ['number', '{n, number} items', { n: 1234567.5 }],
  ['percent', '{n, number, percent}', { n: 0.256 }],
  ['select', '{kind, select, draft {A draft} scheduled {Scheduled} other {Something else}}', { kind: 'scheduled' }],
  ['select other', '{kind, select, draft {A draft} other {Something else}}', { kind: 'xyz' }],
  ['plural', '{count, plural, one {# attempt} other {# attempts}}', { count: 1 }],
  ['plural 5', '{count, plural, one {# attempt} other {# attempts}}', { count: 5 }],
  ['plural =0', '{count, plural, =0 {No posts} one {# post} other {# posts}}', { count: 0 }],
  ['plural offset', '{n, plural, offset:1 =0 {nobody} =1 {one person} one {# other} other {# others}}', { n: 3 }],
  ['ordinal', '{n, selectordinal, one {#st} two {#nd} few {#rd} other {#th}}', { n: 22 }],
  ['nested', '{n, plural, one {{name} has # post} other {{name} has # posts}}', { n: 2, name: 'Ana' }],
  ['select in plural', '{n, plural, other {{g, select, a {A #} other {B #}}}}', { n: 4, g: 'a' }],
  ['pound literal outside plural', 'Item #1', {}],
];

describe('formatIcu matches intl-messageformat', () => {
  for (const locale of ['en', 'ru', 'ar', 'ja', 'de']) {
    it.each(corpus)(`${locale}: %s`, (_name, msg, values) => {
      const want = new IntlMessageFormat(msg, formatTag(locale)).format(values);
      expect(formatIcu(msg, locale, values)).toBe(want);
    });
  }
});

describe('plurals follow CLDR', () => {
  const msg = '{count, plural, one {# пост} few {# поста} many {# постов} other {# поста}}';
  it.each([
    [1, '1 пост'],
    [2, '2 поста'],
    [5, '5 постов'],
    [21, '21 пост'],
  ])('ru %s', (n, want) => expect(formatIcu(msg, 'ru', { count: n })).toBe(want));
  it('ru fraction uses other', () => expect(formatIcu(msg, 'ru', { count: 1.5 })).toBe('1,5 поста'));
  it('ar uses all six categories', () => {
    const ar = '{n, plural, zero {Z} one {O} two {T} few {F} many {M} other {X}}';
    expect([0, 1, 2, 3, 11, 100].map((n) => formatIcu(ar, 'ar', { n }))).toEqual(['Z', 'O', 'T', 'F', 'M', 'X']);
  });
});

describe('rich tags and dates', () => {
  it('plain format keeps the tag content', () => {
    expect(formatIcu('Read the <link>terms</link>.', 'en')).toBe('Read the terms.');
  });
  it('formats dates in the given timezone', () => {
    const d = '2026-10-09T23:30:00Z';
    expect(formatIcu('{d, time, short}', 'en', { d }, 'UTC')).toBe('23:30');
    expect(formatIcu('{d, time, short}', 'en', { d }, 'Asia/Almaty')).toBe('04:30');
  });
});

describe('syntax errors', () => {
  it.each(['{', '{n, plural, one {x}}', '{n, wat, other {x}}', '<b>open', '</b>', 'a } b'])('%s', (m) => {
    expect(() => parseIcu(m)).toThrow(IcuSyntaxError);
  });
  it('a missing value throws', () => {
    expect(() => formatIcu('Hi {name}', 'en', {})).toThrow();
  });
});
