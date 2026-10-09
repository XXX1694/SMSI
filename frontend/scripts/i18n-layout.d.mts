export function listBundles(localeDir: string): string[];
export function strayEntries(localeDir: string): string[];
export function readLocale(localeDir: string): Record<string, object>;
export function catalogBundles(source: string): { list: string[]; typed: string[] };
export function indexBundles(source: string, locale: string): string[];
export function layoutProblems(input: {
  bundles: Record<string, string[]>;
  catalog: { list: string[]; typed: string[] };
  indexes?: Record<string, string[] | null>;
  stray?: Record<string, string[]>;
}): string[];
