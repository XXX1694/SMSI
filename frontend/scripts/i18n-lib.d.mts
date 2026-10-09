export function flatten(obj: object, prefix?: string): Record<string, unknown>;
export function shape(message: string): { args: Set<string>; tags: Set<string> };
export function listSources(dir: string): string[];
export function usedKeys(source: string): string[];
export function literals(source: string): Set<string>;
export function readJson(path: string): object;
export function problems(input: {
  catalogs: Record<string, object>;
  enabled: string[];
  meta?: object;
  sources?: string[];
}): { errors: string[]; warnings: string[] };
export const STYLES: { number: string[]; date: string[]; time: string[] };
