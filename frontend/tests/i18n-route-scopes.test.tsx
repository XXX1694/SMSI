import { act, render, screen } from '@testing-library/react';
import fs from 'node:fs';
import path from 'node:path';
import { Suspense, type ComponentType, type ReactNode } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { FALLBACK, LocaleContext } from '@/i18n/locale-context';
import { useTranslations } from '@/i18n/use-translations';
import type { KeysOf } from '@/i18n/translate';
import { namespacesOfScope, scopesImportedBy, usedNamespaces } from './helpers/used-namespaces';

const APP_DIR = path.resolve(__dirname, '../src/app');

function filesNamed(dir: string, name: string): string[] {
  return fs.readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) return filesNamed(full, name);
    return entry.name === name ? [full] : [];
  });
}

const rel = (file: string) => path.relative(path.resolve(__dirname, '../src'), file).split(path.sep).join('/');

/** The layouts above a page, outermost first, then the page itself. */
function chainOf(pagePath: string): string[] {
  const page = path.join(path.resolve(__dirname, '../src'), pagePath);
  const chain: string[] = [];
  let dir = path.dirname(page);
  while (dir.startsWith(APP_DIR)) {
    const layout = path.join(dir, 'layout.tsx');
    if (fs.existsSync(layout)) chain.unshift(layout);
    dir = path.dirname(dir);
  }
  return [...chain.map(rel), pagePath];
}

const PAGES = filesNamed(APP_DIR, 'page.tsx')
  .map(rel)
  .filter((p) => p !== 'app/page.tsx'); // the redirect to /dashboard renders nothing

const DEMO_SCOPE = 'i18n/scopes/demo.tsx';
const DEMO_BANNER = 'components/demo-banner.tsx';

/** The demo scope exists in the demo build only, so it never counts as loaded for a route. */
function loadedNamespaces(chain: string[]): Set<string> {
  const scopes = ['i18n/i18n-root.tsx', ...chain].flatMap(scopesImportedBy).filter((s) => s !== DEMO_SCOPE);
  return new Set(scopes.flatMap(namespacesOfScope));
}

function usedAlong(chain: string[]): Map<string, Set<string>> {
  const used = new Map<string, Set<string>>();
  for (const file of chain) {
    for (const [ns, files] of usedNamespaces(file, [DEMO_BANNER])) used.set(ns, new Set([...(used.get(ns) ?? []), ...files]));
  }
  return used;
}

describe('every route loads the namespaces its components use', () => {
  it('finds the pages', () => {
    expect(PAGES.length).toBeGreaterThan(15);
  });

  it.each(PAGES)('%s', (page) => {
    const chain = chainOf(page);
    const loaded = loadedNamespaces(chain);
    const missing = [...usedAlong(chain)].filter(([ns]) => !loaded.has(ns));
    // The message names the file that asks, so the fix (a scope in src/i18n/scopes) is obvious.
    expect(
      missing.map(([ns, files]) => `${ns} (used by ${[...files].slice(0, 3).join(', ')})`),
      `${page} renders text from namespaces its scopes do not load (${[...loaded].sort().join(', ') || 'none'})`,
    ).toEqual([]);
  });

  it.each(PAGES)('%s declares no namespace nothing on it uses', (page) => {
    const chain = chainOf(page);
    const used = usedAlong(chain);
    // Only the scopes of the page and its own layouts: the root and the group layouts serve every page below them.
    const own = [...chain.filter((f) => f === page || !/^app\/(layout|\(\w+\)\/layout)\.tsx$/.test(f)).flatMap(scopesImportedBy)].flatMap(namespacesOfScope);
    expect(
      own.filter((ns) => !used.has(ns)),
      `${page} loads namespaces that no component on it uses`,
    ).toEqual([]);
  });

  it('loads the namespaces of the demo banner inside DemoScope, the only place it renders', () => {
    const banner = [...usedNamespaces(DEMO_BANNER).keys()].filter((ns) => !['common', 'errors'].includes(ns));
    expect(namespacesOfScope(DEMO_SCOPE)).toEqual(expect.arrayContaining(banner));
    const layout = fs.readFileSync(path.resolve(__dirname, '../src/app/layout.tsx'), 'utf8');
    expect(layout).toMatch(/<DemoScope>\s*<DemoBanner \/>\s*<\/DemoScope>/);
  });

  it('keeps the root to the namespaces every route needs', () => {
    expect(namespacesOfScope('i18n/scopes/core.tsx')).toEqual(['common', 'errors']);
    expect(scopesImportedBy('i18n/i18n-root.tsx')).toEqual(['i18n/scopes/core.tsx']);
    // The demo banner is the one root-level component with its own namespace, rendered in the demo build only.
    expect(scopesImportedBy('app/layout.tsx')).toEqual(['i18n/scopes/demo.tsx']);
  });
});

// Each scope module, mounted while the locale is already Russian (a client-side navigation to a route): it suspends until
// its bundles have arrived, then shows Russian, never a key.
const SCOPE_NAMES = fs.readdirSync(path.resolve(__dirname, '../src/i18n/scopes')).map((f) => f.replace('.tsx', ''));
const readCatalog = (locale: string, ns: string) =>
  JSON.parse(fs.readFileSync(path.resolve(__dirname, `../messages/${locale}/${ns}.json`), 'utf8')) as Record<string, unknown>;

function firstLeaf(node: Record<string, unknown>, prefix = ''): string {
  for (const [k, v] of Object.entries(node)) {
    if (typeof v === 'string') return prefix + k;
    if (v && typeof v === 'object') return firstLeaf(v as Record<string, unknown>, `${prefix}${k}.`);
  }
  return '';
}
function valueAt(node: Record<string, unknown>, key: string): unknown {
  return key.split('.').reduce<unknown>((cur, part) => (cur as Record<string, unknown> | undefined)?.[part], node);
}

function Probe({ keys }: { keys: string[] }) {
  const t = useTranslations();
  return (
    <ul>
      {keys.map((k) => (
        <li key={k} data-testid={k}>
          {t(k as KeysOf<undefined>)}
        </li>
      ))}
    </ul>
  );
}

describe('a route scope mounted in Russian', () => {
  afterEach(() => vi.restoreAllMocks());

  it.each(SCOPE_NAMES)('%s suspends, then renders Russian', async (name) => {
    const logged = vi.spyOn(console, 'error').mockImplementation(() => {});
    const mod = (await import(`../src/i18n/scopes/${name}`)) as Record<string, ComponentType<{ children: ReactNode }>>;
    const Scope = Object.values(mod)[0] as ComponentType<{ children: ReactNode }>;
    const namespaces = namespacesOfScope(`i18n/scopes/${name}.tsx`);
    expect(namespaces.length).toBeGreaterThan(0);
    const keys = namespaces.map((ns) => `${ns}.${firstLeaf(readCatalog('en', ns))}`);
    let suspended = false;
    function Fallback() {
      suspended = true;
      return <p>loading</p>;
    }
    await act(async () => {
      render(
        <LocaleContext.Provider value={{ ...FALLBACK, locale: 'ru' }}>
          <Suspense fallback={<Fallback />}>
            <Scope>
              <Probe keys={keys} />
            </Scope>
          </Suspense>
        </LocaleContext.Provider>,
      );
    });
    expect(suspended).toBe(true);
    for (const key of keys) {
      const [ns, ...rest] = key.split('.');
      const expected = valueAt(readCatalog('ru', ns ?? ''), rest.join('.'));
      expect(typeof expected).toBe('string');
      expect(await screen.findByTestId(key)).toHaveTextContent(expected as string);
    }
    expect(logged).not.toHaveBeenCalled();
  });
});
