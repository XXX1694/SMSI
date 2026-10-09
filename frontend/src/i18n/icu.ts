/**
 * A small ICU MessageFormat runtime: arguments, number/date/time, plural, selectordinal, select, `#` and rich tags.
 * That is the whole subset translation-process.md allows. It replaces next-intl's formatter on the client because
 * `intl-messageformat` with its parser costs about 14 kB gzipped on every route (measured, see CHANGELOG), and this
 * is under 2 kB. `scripts/i18n-check.mjs` validates every catalog with the official FormatJS parser, and
 * `tests/i18n-icu.test.ts` asserts that both engines print the same text, so the two cannot drift apart.
 */
import { formatTag } from '@/i18n/locales';

export type Node =
  | string
  | { t: 'arg'; name: string; fmt?: string; style?: string }
  | { t: 'pound' }
  | { t: 'select'; name: string; opts: Record<string, Node[]> }
  | { t: 'plural'; name: string; ordinal: boolean; offset: number; opts: Record<string, Node[]> }
  | { t: 'tag'; name: string; children: Node[] };

export class IcuSyntaxError extends Error {}

const SPECIAL = '{}<>#';

class Parser {
  private i = 0;
  constructor(private readonly s: string) {}

  parse(): Node[] {
    const nodes = this.nodes(false, null);
    if (this.i < this.s.length) throw new IcuSyntaxError(`unexpected "${this.s[this.i]}" at ${this.i}`);
    return nodes;
  }

  private nodes(inPlural: boolean, closeTag: string | null): Node[] {
    const out: Node[] = [];
    let text = '';
    const flush = () => {
      if (text) out.push(text);
      text = '';
    };
    const s = this.s;
    while (this.i < s.length) {
      const c = s[this.i] as string;
      if (c === "'") {
        const next = s[this.i + 1];
        if (next === "'") {
          text += "'";
          this.i += 2;
        } else if (next !== undefined && SPECIAL.includes(next) && (next !== '#' || inPlural)) {
          // Quoted literal: up to the next lone apostrophe.
          this.i++;
          while (this.i < s.length) {
            if (s[this.i] === "'") {
              if (s[this.i + 1] === "'") {
                text += "'";
                this.i += 2;
                continue;
              }
              this.i++;
              break;
            }
            text += s[this.i++];
          }
        } else {
          text += c;
          this.i++;
        }
      } else if (c === '{') {
        flush();
        out.push(this.argument());
      } else if (c === '}') {
        break;
      } else if (c === '#' && inPlural) {
        flush();
        out.push({ t: 'pound' });
        this.i++;
      } else if (c === '<' && s[this.i + 1] === '/') {
        const end = s.indexOf('>', this.i);
        const name = end < 0 ? '' : s.slice(this.i + 2, end);
        if (closeTag === null || name !== closeTag) throw new IcuSyntaxError(`unexpected </${name}>`);
        break;
      } else if (c === '<' && /[A-Za-z]/.test(s[this.i + 1] ?? '')) {
        flush();
        out.push(this.tag(inPlural));
      } else {
        text += c;
        this.i++;
      }
    }
    flush();
    return out;
  }

  private tag(inPlural: boolean): Node {
    const end = this.s.indexOf('>', this.i);
    if (end < 0) throw new IcuSyntaxError('unterminated tag');
    const name = this.s.slice(this.i + 1, end);
    this.i = end + 1;
    const children = this.nodes(inPlural, name);
    if (!this.s.startsWith(`</${name}>`, this.i)) throw new IcuSyntaxError(`missing </${name}>`);
    this.i += name.length + 3;
    return { t: 'tag', name, children };
  }

  private ws() {
    while (/\s/.test(this.s[this.i] ?? '')) this.i++;
  }

  private word(): string {
    this.ws();
    const m = /^[^\s,{}]+/.exec(this.s.slice(this.i));
    if (!m) throw new IcuSyntaxError(`expected a name at ${this.i}`);
    this.i += m[0].length;
    return m[0];
  }

  private argument(): Node {
    this.i++; // {
    const name = this.word();
    this.ws();
    if (this.s[this.i] === '}') {
      this.i++;
      return { t: 'arg', name };
    }
    if (this.s[this.i] !== ',') throw new IcuSyntaxError(`expected "," or "}" at ${this.i}`);
    this.i++;
    const kind = this.word();
    this.ws();
    if (kind === 'number' || kind === 'date' || kind === 'time') {
      let style: string | undefined;
      if (this.s[this.i] === ',') {
        this.i++;
        style = this.word();
        this.ws();
      }
      this.expect('}');
      return { t: 'arg', name, fmt: kind, style };
    }
    if (kind !== 'plural' && kind !== 'selectordinal' && kind !== 'select') throw new IcuSyntaxError(`unknown type "${kind}"`);
    this.expect(',');
    let offset = 0;
    this.ws();
    const off = /^offset:\s*(\d+)/.exec(this.s.slice(this.i));
    if (off) {
      offset = Number(off[1]);
      this.i += off[0].length;
    }
    const opts: Record<string, Node[]> = {};
    for (;;) {
      this.ws();
      if (this.s[this.i] === '}') break;
      const key = this.word();
      this.ws();
      this.expect('{');
      opts[key] = this.nodes(kind !== 'select', null); // like FormatJS, `#` is literal in a select, even inside a plural
      this.expect('}');
    }
    this.expect('}');
    if (!('other' in opts)) throw new IcuSyntaxError(`"${name}" needs an "other" option`);
    return kind === 'select' ? { t: 'select', name, opts } : { t: 'plural', name, ordinal: kind === 'selectordinal', offset, opts };
  }

  private expect(ch: string) {
    this.ws();
    if (this.s[this.i] !== ch) throw new IcuSyntaxError(`expected "${ch}" at ${this.i}`);
    this.i++;
  }
}

const cache = new Map<string, Node[]>();

export function parseIcu(message: string): Node[] {
  let nodes = cache.get(message);
  if (!nodes) {
    nodes = new Parser(message).parse();
    cache.set(message, nodes);
  }
  return nodes;
}

export type IcuValue = string | number | boolean | Date | null | undefined;
export interface FormatContext<R> {
  locale: string;
  timeZone?: string;
  values: Record<string, IcuValue | ((chunks: R[]) => R)>;
}

const DATE_STYLES = new Set(['short', 'medium', 'long', 'full']);

function scalar<R>(ctx: FormatContext<R>, name: string): IcuValue {
  const v = ctx.values[name];
  if (typeof v === 'function') throw new Error(`"${name}" is a tag handler, not a value`);
  return v;
}

/** Walks the message. `R` is the type of a rich chunk (a React node), `string` for plain text. */
export function formatNodes<R = string>(nodes: Node[], ctx: FormatContext<R>, pound?: string): (string | R)[] {
  const out: (string | R)[] = [];
  const tag = formatTag(ctx.locale);
  for (const n of nodes) {
    if (typeof n === 'string') out.push(n);
    else if (n.t === 'pound') out.push(pound ?? '#');
    else if (n.t === 'arg') {
      const v = scalar(ctx, n.name);
      if (v === undefined || v === null) throw new Error(`missing value for "${n.name}"`);
      if (n.fmt === 'number') {
        const opts: Intl.NumberFormatOptions = n.style === 'percent' ? { style: 'percent' } : {};
        out.push(new Intl.NumberFormat(tag, opts).format(Number(v)));
      } else if (n.fmt === 'date' || n.fmt === 'time') {
        const style = n.style && DATE_STYLES.has(n.style) ? (n.style as 'short') : 'medium';
        const opts: Intl.DateTimeFormatOptions = n.fmt === 'date' ? { dateStyle: style } : { timeStyle: style };
        out.push(new Intl.DateTimeFormat(tag, { ...opts, timeZone: ctx.timeZone }).format(new Date(v as string | number | Date)));
      } else out.push(String(v));
    } else if (n.t === 'select') {
      const key = String(scalar(ctx, n.name));
      out.push(...formatNodes(n.opts[key] ?? (n.opts.other as Node[]), ctx));
    } else if (n.t === 'plural') {
      const raw = Number(scalar(ctx, n.name));
      const num = raw - n.offset;
      const exact = n.opts[`=${raw}`];
      const category = new Intl.PluralRules(ctx.locale, { type: n.ordinal ? 'ordinal' : 'cardinal' }).select(num);
      const body = exact ?? n.opts[category] ?? (n.opts.other as Node[]);
      out.push(...formatNodes(body, ctx, new Intl.NumberFormat(tag).format(num)));
    } else {
      const children = formatNodes(n.children, ctx, pound);
      const handler = ctx.values[n.name];
      if (typeof handler === 'function') out.push(handler(children as R[]));
      else out.push(...children);
    }
  }
  return out;
}

export function formatIcu(message: string, locale: string, values: Record<string, IcuValue> = {}, timeZone?: string): string {
  return formatNodes<string>(parseIcu(message), { locale, timeZone, values }).join('');
}
