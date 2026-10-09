export function flatten(obj: object, prefix?: string): Record<string, unknown>;
export function shape(message: string): { args: Set<string>; tags: Set<string> };
export function listSources(dir: string): string[];
export function usedKeys(source: string): string[];
export function usedPrefixes(source: string): string[];
export function literals(source: string): Set<string>;
export function readJson(path: string): object;
export function problems(input: {
  catalogs: Record<string, object>;
  enabled: string[];
  meta?: object;
  sources?: string[];
}): { errors: string[]; warnings: string[] };
export const STYLES: { number: string[]; date: string[]; time: string[] };
export function listBundles(localeDir: string): string[];
export function readLocale(localeDir: string): Record<string, object>;
export function catalogBundles(source: string): { list: string[]; typed: string[] };
export function layoutProblems(input: { bundles: Record<string, string[]>; catalog: { list: string[]; typed: string[] } }): string[];
