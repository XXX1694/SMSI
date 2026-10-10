import { gzipSync } from 'node:zlib';
import { describe, expect, it } from 'vitest';
import { compareBudgets, filesContaining, formatTable, gzipBytes, pageKeyFor, routeScripts, toKb } from '../scripts/bundle-budget-lib.mjs';

// A trimmed copy of the shape Next 15 writes to .next/app-build-manifest.json and .next/build-manifest.json.
const manifests = {
  buildManifest: {
    polyfillFiles: ['static/chunks/polyfills-aaa.js'],
    rootMainFiles: ['static/chunks/webpack-1.js', 'static/chunks/framework-2.js', 'static/chunks/main-app-3.js'],
  },
  appManifest: {
    pages: {
      '/layout': ['static/chunks/webpack-1.js', 'static/chunks/main-app-3.js', 'static/css/root.css', 'static/chunks/app/layout-4.js'],
      '/(auth)/layout': ['static/chunks/webpack-1.js', 'static/chunks/auth-5.js', 'static/chunks/app/(auth)/layout-6.js'],
      '/(auth)/login/page': ['static/chunks/webpack-1.js', 'static/chunks/app/(auth)/login/page-7.js'],
      '/(app)/layout': ['static/chunks/app-shell-8.js', 'static/chunks/app/(app)/layout-9.js'],
      '/(app)/posts/page': ['static/chunks/app/(app)/posts/page-10.js'],
      '/(app)/posts/[id]/page': ['static/chunks/app/(app)/posts/[id]/page-11.js'],
      '/(app)/developer/layout': ['static/chunks/dev-12.js'],
      '/(app)/developer/page': ['static/chunks/app/(app)/developer/page-13.js'],
      '/page': ['static/chunks/app/page-14.js'],
    },
  },
};

describe('pageKeyFor', () => {
  it('finds the page entry behind a route group', () => {
    expect(pageKeyFor('/login', manifests.appManifest.pages)).toBe('/(auth)/login/page');
    expect(pageKeyFor('/', manifests.appManifest.pages)).toBe('/page');
  });

  it('does not mistake a dynamic sibling for the route', () => {
    expect(pageKeyFor('/posts', manifests.appManifest.pages)).toBe('/(app)/posts/page');
  });

  it('fails loudly for a route that has no page', () => {
    expect(() => pageKeyFor('/nope', manifests.appManifest.pages)).toThrow(/\/nope/);
  });
});

describe('routeScripts', () => {
  it('joins root main files, root layout, group layout and page, without css or polyfills, each once', () => {
    expect(routeScripts('/login', manifests)).toEqual([
      'static/chunks/app/(auth)/layout-6.js',
      'static/chunks/app/(auth)/login/page-7.js',
      'static/chunks/app/layout-4.js',
      'static/chunks/auth-5.js',
      'static/chunks/framework-2.js',
      'static/chunks/main-app-3.js',
      'static/chunks/webpack-1.js',
    ]);
  });

  it('includes a nested layout when the route has one, and no layout from another group', () => {
    const files = routeScripts('/developer', manifests);
    expect(files).toContain('static/chunks/dev-12.js');
    expect(files).toContain('static/chunks/app/(app)/layout-9.js');
    expect(files).not.toContain('static/chunks/auth-5.js');
    expect(routeScripts('/posts', manifests)).not.toContain('static/chunks/dev-12.js');
  });
});

describe('compareBudgets', () => {
  it('flags only the routes above their budget; equal passes', () => {
    const rows = compareBudgets({ '/a': 150_000, '/b': 150_001 }, { '/a': 150, '/b': 150 });
    expect(rows.map((r) => [r.route, r.over])).toEqual([['/a', false], ['/b', true]]);
  });

  it('refuses a budget for a route that was not measured', () => {
    expect(() => compareBudgets({}, { '/a': 1 })).toThrow(/\/a/);
  });

  it('prints a table naming the overshoot', () => {
    const table = formatTable(compareBudgets({ '/a': 152_300 }, { '/a': 150 }));
    expect(table).toContain('/a');
    expect(table).toContain('OVER by 2.3 kB');
  });
});

describe('helpers', () => {
  it('measures gzip at level 6 and converts to kB of 1000 bytes', () => {
    const data = Buffer.from('steerpost '.repeat(500));
    expect(gzipBytes(data)).toBe(gzipSync(data, { level: 6 }).length);
    expect(toKb(1234)).toBe(1.23);
  });

  it('lists the files that contain a marker', () => {
    expect(filesContaining({ a: 'x skipToContent y', b: 'nothing' }, 'skipToContent')).toEqual(['a']);
  });
});
