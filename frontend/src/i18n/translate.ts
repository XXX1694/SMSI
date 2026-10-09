import { formatNodes, parseIcu, type FormatContext, type IcuValue } from '@/i18n/icu';
import type { Catalog } from '@/i18n/pseudo';
import type en from '../../messages/en.json';

type Messages = typeof en;
/** Dotted paths of every string in a catalog: `nav.dashboard`. */
type Paths<T> = T extends string ? never : { [K in keyof T & string]: T[K] extends string ? K : `${K}.${Paths<T[K]>}` }[keyof T & string];
/** Dotted paths of every object node in a catalog: `nav`. */
export type Namespace = Exclude<{ [K in keyof Messages & string]: Messages[K] extends string ? never : K }[keyof Messages & string], never>;
type Sub<T, P extends string> = P extends `${infer H}.${infer R}` ? (H extends keyof T ? Sub<T[H], R> : never) : P extends keyof T ? T[P] : never;
export type KeysOf<N extends Namespace | undefined> = N extends Namespace ? Paths<Sub<Messages, N>> : Paths<Messages>;

export type Values = Record<string, IcuValue>;

export interface Translator<K extends string = string> {
  (key: K, values?: Values): string;
  /** Like `t`, but tag handlers (`<b>…</b>`) may return any value, e.g. a React element. */
  rich<R>(key: K, values: Record<string, IcuValue | ((chunks: R[]) => R)>): (string | R)[];
}

export interface TranslatorConfig {
  locale: string;
  messages: Catalog;
  timeZone?: string;
  /** Called for a key that is in no catalog. Default: `console.error` outside production. */
  onMissing?: (path: string) => void;
}

function lookup(messages: Catalog, path: string): string | undefined {
  let cur: string | Catalog | undefined = messages;
  for (const part of path.split('.')) {
    if (typeof cur !== 'object' || cur === null) return undefined;
    cur = cur[part];
  }
  return typeof cur === 'string' ? cur : undefined;
}

export function createTranslator<N extends Namespace | undefined = undefined>(config: TranslatorConfig, ns?: N): Translator<KeysOf<N>> {
  const prefix = ns ? `${ns}.` : '';
  const missing =
    config.onMissing ??
    ((p: string) => {
      if (process.env.NODE_ENV !== 'production') console.error(`MISSING_MESSAGE: ${p} (${config.locale})`);
    });
  const run = (key: string, values: FormatContext<unknown>['values']): (string | unknown)[] => {
    const path = prefix + key;
    const msg = lookup(config.messages, path);
    if (msg === undefined) {
      missing(path);
      return [path];
    }
    try {
      return formatNodes<unknown>(parseIcu(msg), { locale: config.locale, timeZone: config.timeZone, values });
    } catch (e) {
      missing(`${path}: ${(e as Error).message}`);
      return [path];
    }
  };
  const t = (key: string, values: Values = {}) => run(key, values).join('');
  const rich = (key: string, values: FormatContext<unknown>['values']) => run(key, values);
  return Object.assign(t, { rich }) as unknown as Translator<KeysOf<N>>;
}
