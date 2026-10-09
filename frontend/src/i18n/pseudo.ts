/**
 * The en-XA pseudo-locale: accents every letter, makes text about 35 % longer and wraps it in brackets. Hard-coded
 * strings stay plain English, so they show up at a glance; clipped or overflowing layouts show up too. It is generated
 * from the English catalog at runtime and never committed. ICU syntax is kept intact by working on the parsed message.
 */
import { parse, TYPE, type MessageFormatElement } from '@formatjs/icu-messageformat-parser';
import { printAST } from '@formatjs/icu-messageformat-parser/printer.js';

const MAP: Record<string, string> = {
  a: 'à', b: 'ƀ', c: 'ç', d: 'ď', e: 'é', f: 'ƒ', g: 'ĝ', h: 'ĥ', i: 'î', j: 'ĵ', k: 'ķ', l: 'ĺ', m: 'ɱ', n: 'ñ', o: 'ö',
  p: 'þ', q: 'ǫ', r: 'ŕ', s: 'š', t: 'ţ', u: 'û', v: 'ṽ', w: 'ŵ', x: 'ẋ', y: 'ý', z: 'ž',
  A: 'À', B: 'Ɓ', C: 'Ç', D: 'Ď', E: 'É', F: 'Ƒ', G: 'Ĝ', H: 'Ĥ', I: 'Î', J: 'Ĵ', K: 'Ķ', L: 'Ĺ', M: 'Ṁ', N: 'Ñ', O: 'Ö',
  P: 'Þ', Q: 'Ǫ', R: 'Ŕ', S: 'Š', T: 'Ţ', U: 'Û', V: 'Ṽ', W: 'Ŵ', X: 'Ẋ', Y: 'Ý', Z: 'Ž',
};
const VOWELS = new Set('aeiouAEIOU');
const EXPANSION = 0.35;

/** Accents `text` and doubles vowels (then appends dots) until it is about 35 % longer. */
export function pseudoText(text: string): string {
  if (!/[A-Za-z]/.test(text)) return text;
  let extra = Math.ceil([...text].length * EXPANSION);
  let out = '';
  for (const ch of text) {
    const accented = MAP[ch] ?? ch;
    out += accented;
    if (extra > 0 && VOWELS.has(ch)) {
      out += accented;
      extra--;
    }
  }
  return out + '·'.repeat(Math.max(0, extra));
}

function walk(els: MessageFormatElement[]): MessageFormatElement[] {
  return els.map((el): MessageFormatElement => {
    switch (el.type) {
      case TYPE.literal:
        return { ...el, value: pseudoText(el.value) };
      case TYPE.plural:
      case TYPE.select:
        return {
          ...el,
          options: Object.fromEntries(Object.entries(el.options).map(([k, v]) => [k, { value: walk(v.value) }])),
        } as MessageFormatElement;
      case TYPE.tag:
        return { ...el, children: walk(el.children) };
      default:
        return el;
    }
  });
}

/** One ICU message → its pseudo version, in brackets. A message that does not parse is returned wrapped, unchanged. */
export function pseudoMessage(message: string): string {
  // A separator such as ", " has nothing to accent; brackets around it would show up inside lists.
  if (!/[A-Za-z{<]/.test(message)) return message;
  try {
    return `[${printAST(walk(parse(message)))}]`;
  } catch {
    return `[${message}]`;
  }
}

export interface Catalog {
  [key: string]: string | Catalog;
}

export function pseudoCatalog(en: Catalog): Catalog {
  const out: Catalog = {};
  for (const [k, v] of Object.entries(en)) out[k] = typeof v === 'string' ? pseudoMessage(v) : pseudoCatalog(v);
  return out;
}
