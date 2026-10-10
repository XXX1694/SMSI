import fs from 'node:fs';
import path from 'node:path';

/**
 * Which message namespaces the code behind an entry file can ask for, found by reading the source: `useTranslations('ns')`
 * and string literals that are catalog keys (`'posts.untitled'`, or a template whose prefix is one: `` `posts.status.${s}` ``).
 *
 * Components and pages count as a whole file. Modules under src/lib, src/i18n and src/hooks.ts count per declaration,
 * and only the ones the importer names (plus what those reference in the same file), so importing `lib/format` for
 * `joinList` does not pull in the keys of `postTitle`. A key built from pieces at run time is invisible here; the dev
 * guard in src/i18n/translate.ts and the browser smoke check catch those.
 */
const ROOT = path.resolve(__dirname, '../..');
const SRC = path.join(ROOT, 'src');
const ALL = '*';

function catalogKeys(): string[] {
  const keys: string[] = [];
  const walk = (node: unknown, prefix: string) => {
    for (const [k, v] of Object.entries(node as Record<string, unknown>)) {
      if (typeof v === 'string') keys.push(prefix + k);
      else walk(v, `${prefix}${k}.`);
    }
  };
  const dir = path.join(ROOT, 'messages/en');
  for (const file of fs.readdirSync(dir)) {
    walk(JSON.parse(fs.readFileSync(path.join(dir, file), 'utf8')), `${file.replace('.json', '')}.`);
  }
  return keys;
}

const KEYS = catalogKeys();
const NAMESPACES = new Set(KEYS.map((k) => k.split('.')[0]));

/** A capture group that is known to have matched. */
const group = (m: RegExpMatchArray, i: number): string => m[i] ?? '';

function isKey(literal: string): boolean {
  const cut = literal.indexOf('${');
  if (cut === -1) return KEYS.includes(literal);
  const prefix = literal.slice(0, cut);
  return prefix.includes('.') && KEYS.some((k) => k.startsWith(prefix));
}

function stripComments(source: string): string {
  return source.replace(/\/\*[\s\S]*?\*\//g, '').replace(/^\s*\/\/.*$/gm, '');
}

export function namespacesInText(source: string): Set<string> {
  const text = stripComments(source);
  const out = new Set<string>();
  for (const m of text.matchAll(/useTranslations\(\s*['"]([a-zA-Z]+)/g)) if (NAMESPACES.has(group(m, 1))) out.add(group(m, 1));
  for (const m of text.matchAll(/(['"`])([a-zA-Z]+\.[^'"`\n]*)\1/g)) {
    if (isKey(group(m, 2))) out.add(group(m, 2).split('.')[0] ?? '');
  }
  return out;
}

function resolveModule(from: string, spec: string): string | null {
  let base: string;
  if (spec.startsWith('@/')) base = path.join(SRC, spec.slice(2));
  else if (spec.startsWith('.')) base = path.resolve(path.dirname(from), spec);
  else return null;
  for (const candidate of [`${base}.tsx`, `${base}.ts`, path.join(base, 'index.tsx'), path.join(base, 'index.ts')]) {
    if (fs.existsSync(candidate)) return candidate;
  }
  return null;
}

/** Modules read per declaration: everything that is not a component, a page or a layout. */
function isDeclarationModule(file: string): boolean {
  const rel = path.relative(SRC, file);
  return rel === 'hooks.ts' || rel.startsWith(`lib${path.sep}`) || rel.startsWith(`i18n${path.sep}`);
}

interface Import {
  module: string;
  /** Imported names, or ALL for a default, namespace or side-effect import and for `export … from`. */
  names: string[];
  /** Local alias → imported name. */
  alias: Map<string, string>;
}

function parseImports(file: string, text: string): Import[] {
  const out: Import[] = [];
  for (const m of text.matchAll(/(?:^|\n)\s*(import|export)\s+(type\s+)?([^;'"]*?)\s*(?:from\s*)?['"]([^'"]+)['"]/g)) {
    if (m[2]) continue;
    const resolved = resolveModule(file, group(m, 4));
    if (!resolved) continue;
    const clause = group(m, 3);
    const braces = /\{([^}]*)\}/.exec(clause);
    const alias = new Map<string, string>();
    let names: string[] = [ALL];
    if (m[1] === 'import' && braces && !/^\s*[\w$]+\s*,/.test(clause) && !clause.includes('*')) {
      names = [];
      for (const part of (braces[1] ?? '').split(',')) {
        const piece = part.trim();
        if (!piece || piece.startsWith('type ')) continue;
        const [imported = '', local = imported] = piece.split(/\s+as\s+/);
        names.push(imported);
        alias.set(local, imported);
      }
    }
    out.push({ module: resolved, names, alias });
  }
  for (const m of text.matchAll(/import\(\s*['"]([^'"]+)['"]\s*\)/g)) {
    const resolved = resolveModule(file, group(m, 1));
    if (resolved) out.push({ module: resolved, names: [ALL], alias: new Map() });
  }
  return out;
}

interface Chunk {
  name: string;
  text: string;
}

/** A module split at its top-level declarations (they start in column 0). */
function chunksOf(text: string): Chunk[] {
  const starts = [...text.matchAll(/^(?:export\s+)?(?:default\s+)?(?:async\s+)?(?:function\*?|const|let|class|enum)\s+([\w$]+)/gm)];
  return starts.map((m, i) => ({ name: group(m, 1), text: text.slice(m.index, starts[i + 1]?.index ?? text.length) }));
}

type Used = Map<string, Set<string>>;

function add(used: Used, namespaces: Set<string>, file: string) {
  for (const ns of namespaces) {
    if (!used.has(ns)) used.set(ns, new Set());
    used.get(ns)?.add(path.relative(SRC, file));
  }
}

/**
 * The namespaces needed by `entry` (a path under src/) and everything it imports, each with the files that ask for it.
 */
export function usedNamespaces(entry: string, exclude: string[] = []): Used {
  const used: Used = new Map();
  const done = new Set<string>();
  const queue: [string, string[]][] = [[path.join(SRC, entry), [ALL]]];
  while (queue.length) {
    const [file, names] = queue.pop() as [string, string[]];
    const key = `${file}|${[...names].sort().join(',')}`;
    if (done.has(key)) continue;
    done.add(key);
    // The demo backend is English seed data and audit event names (allow-listed for i18n:literals), not UI text.
    if (path.relative(SRC, file).startsWith(`lib${path.sep}demo${path.sep}`)) continue;
    if (exclude.includes(path.relative(SRC, file).split(path.sep).join('/'))) continue;
    const text = stripComments(fs.readFileSync(file, 'utf8'));
    const imports = parseImports(file, text);
    if (!isDeclarationModule(file) || names.includes(ALL)) {
      add(used, namespacesInText(text), file);
      for (const imp of imports) queue.push([imp.module, imp.names]);
      continue;
    }
    const chunks = chunksOf(text);
    const wanted = new Set(names);
    const local = new Set<string>(chunks.map((c) => c.name));
    for (let grew = true; grew; ) {
      grew = false;
      for (const chunk of chunks.filter((c) => wanted.has(c.name))) {
        for (const token of chunk.text.match(/[\w$]+/g) ?? []) {
          if (local.has(token) && !wanted.has(token)) {
            wanted.add(token);
            grew = true;
          }
        }
      }
    }
    for (const chunk of chunks.filter((c) => wanted.has(c.name))) {
      add(used, namespacesInText(chunk.text), file);
      const tokens = new Set(chunk.text.match(/[\w$]+/g) ?? []);
      for (const imp of imports) {
        const from = imp.names.includes(ALL) ? [ALL] : imp.names.filter((n) => tokens.has(n) || [...imp.alias].some(([l, i]) => i === n && tokens.has(l)));
        if (from.length) queue.push([imp.module, from]);
      }
    }
  }
  return used;
}

/** The namespaces a scope module (src/i18n/scopes/*.tsx) loads: the English JSON files it imports. */
export function namespacesOfScope(scopeFile: string): string[] {
  const text = fs.readFileSync(path.join(SRC, scopeFile), 'utf8');
  return [...text.matchAll(/messages\/en\/([a-zA-Z]+)\.json/g)].map((m) => group(m, 1));
}

/** The scope modules a route file renders, by their import path. */
export function scopesImportedBy(entry: string): string[] {
  const text = fs.readFileSync(path.join(SRC, entry), 'utf8');
  return [...text.matchAll(/from\s+['"]@\/i18n\/scopes\/([\w-]+)['"]/g)].map((m) => `i18n/scopes/${group(m, 1)}.tsx`);
}
