import { parse, TYPE, type MessageFormatElement } from '@formatjs/icu-messageformat-parser';
import { IntlMessageFormat } from 'intl-messageformat';
import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';
import { formatIcu, IcuSyntaxError, parseIcu, SUPPORTED_STYLES } from '@/i18n/icu';
import { formatTag } from '@/i18n/locales';
import { flatten, shape, STYLES } from '../scripts/i18n-lib.mjs';

type V = Record<string, string | number | Date>;
const D = new Date('2026-10-09T23:30:00Z');
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
  ['number integer', '{n, number, integer}', { n: 3.7 }],
  ['date default', '{d, date}', { d: D }],
  ['time default', '{d, time}', { d: D }],
  ...['short', 'medium', 'long', 'full'].flatMap((st): [string, string, V][] => [
    [`date ${st}`, `{d, date, ${st}}`, { d: D }],
    [`time ${st}`, `{d, time, ${st}}`, { d: D }],
  ]),
];

const TZ = 'UTC';
// FormatJS has no per-call timezone, so pin the process zone; our runtime gets the same zone explicitly.
process.env.TZ = TZ;
const LOCALES_UNDER_TEST = ['en', 'ru', 'ar', 'ja', 'de', 'en-XA'];

describe('formatIcu matches intl-messageformat', () => {
  for (const locale of LOCALES_UNDER_TEST) {
    it.each(corpus)(`${locale}: %s`, (_name, msg, values) => {
      const want = new IntlMessageFormat(msg, formatTag(locale)).format(values);
      expect(formatIcu(msg, locale, values, TZ)).toBe(want);
    });
  }
});

/** Values for every argument of a parsed message: numbers for plural/number, a date for date/time, each select key. */
function sampleValues(msg: string, n: number): V[] {
  const out: V[] = [{}];
  const walk = (els: MessageFormatElement[]) => {
    for (const el of els) {
      if (el.type === TYPE.plural) for (const o of Object.values(el.options)) walk(o.value);
      if (el.type === TYPE.select) for (const o of Object.values(el.options)) walk(o.value);
      if (el.type === TYPE.tag) walk(el.children);
    }
  };
  const ast = parse(msg);
  walk(ast);
  const collect = (els: MessageFormatElement[], acc: V[]) => {
    for (const el of els) {
      if (el.type === TYPE.argument) acc.forEach((v) => (v[el.value] = 'X'));
      else if (el.type === TYPE.number) acc.forEach((v) => (v[el.value] = n + 0.5));
      else if (el.type === TYPE.date || el.type === TYPE.time) acc.forEach((v) => (v[el.value] = D));
      else if (el.type === TYPE.plural) {
        acc.forEach((v) => (v[el.value] = n));
        for (const o of Object.values(el.options)) collect(o.value, acc);
      } else if (el.type === TYPE.select) {
        const keys = Object.keys(el.options);
        // one run per option, so every branch is formatted
        const base = acc.map((v) => ({ ...v }));
        acc.length = 0;
        for (const k of keys) for (const b of base) acc.push({ ...b, [el.value]: k });
        for (const o of Object.values(el.options)) collect(o.value, acc);
      } else if (el.type === TYPE.tag) collect(el.children, acc);
    }
  };
  collect(ast, out);
  return out;
}

/** Rich tag names anywhere in a message, including inside plural and select options. */
function tagNames(el: MessageFormatElement): string[] {
  if (el.type === TYPE.tag) return [el.value, ...el.children.flatMap(tagNames)];
  if (el.type === TYPE.plural || el.type === TYPE.select) return Object.values(el.options).flatMap((o) => o.value.flatMap(tagNames));
  return [];
}

function catalogMessages(): [string, string][] {
  const out: [string, string][] = [];
  const dir = join(__dirname, '..', 'messages');
  for (const f of readdirSync(dir)) {
    if (f === 'meta.json') continue;
    const flat = flatten(JSON.parse(readFileSync(join(dir, f), 'utf8')));
    for (const [k, v] of Object.entries(flat)) out.push([`${f}:${k}`, String(v)]);
  }
  return out;
}

describe('every catalog message formats exactly like FormatJS', () => {
  const messages = catalogMessages();
  it('has messages to check', () => expect(messages.length).toBeGreaterThan(0));
  for (const locale of LOCALES_UNDER_TEST) {
    it(`${locale}: all catalog messages, plural samples 0 1 2 5 1.5 21`, () => {
      for (const [id, msg] of messages) {
        for (const n of [0, 1, 2, 5, 1.5, 21]) {
          for (const values of sampleValues(msg, n)) {
            const handlers = Object.fromEntries([...parse(msg).flatMap(tagNames)].map((t) => [t, (c: string[]) => c.join('')]));
            const want = [new IntlMessageFormat(msg, formatTag(locale)).format({ ...values, ...handlers })].flat().join('');
            expect(formatIcu(msg, locale, values, TZ), `${id} ${JSON.stringify(values)}`).toBe(want);
          }
        }
      }
    });
  }
});

describe('unsupported styles are rejected, not mis-formatted', () => {
  it.each(['{n, number, ::currency/EUR}', '{n, number, currency}', '{d, date, yyyy}', '{d, time, ::hhmm}'])('%s', (m) => {
    expect(() => parseIcu(m)).toThrow(IcuSyntaxError);
    expect(() => shape(m)).toThrow();
  });
  it('the check script and the runtime allow the same styles', () => {
    expect(STYLES).toEqual(SUPPORTED_STYLES);
  });
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
    expect(formatIcu('{d, time, short}', 'ru', { d }, 'UTC')).toBe('23:30');
    expect(formatIcu('{d, time, short}', 'ru', { d }, 'Asia/Almaty')).toBe('04:30');
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
