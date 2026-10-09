import { describe, expect, it, vi } from 'vitest';
import { BUNDLES } from '@/i18n/catalog';
import en from '@/i18n/en-all';
import { loadBundles, mergeMessages } from '@/i18n/messages';
import type { Catalog } from '@/i18n/pseudo';

const english = en as unknown as Catalog;

describe('locale loader picks namespaces out of the locale chunk', () => {
  it('English is the static catalog and has every bundle', () => {
    expect(Object.keys(en)).toEqual([...BUNDLES]);
  });

  it('returns only the requested namespaces, translated', async () => {
    const ru = await loadBundles('ru', ['nav', 'errors'], english);
    expect(Object.keys(ru ?? {}).sort()).toEqual(['errors', 'nav']);
    expect((ru?.nav as Catalog).dashboard).not.toBe(en.nav.dashboard);
  });

  it('a locale with no bundle files yields nothing to merge, so English stays', async () => {
    const ar = await loadBundles('ar', ['nav'], english);
    expect(mergeMessages({ nav: english.nav as Catalog }, ar ?? {})).toEqual({ nav: english.nav });
  });

  it('en needs nothing; the pseudo-locale is generated from the given English', async () => {
    expect(await loadBundles('en', ['nav'], english)).toEqual({});
    const pseudo = await loadBundles('en-XA', ['nav'], english);
    expect((pseudo?.nav as Catalog).dashboard).toMatch(/^\[/);
  });

  it('caches per locale and namespaces, and marks the promise settled so use() does not suspend', async () => {
    const first = loadBundles('de', ['nav'], english);
    expect(loadBundles('de', ['nav'], english)).toBe(first);
    await first;
    expect((first as unknown as { status: string }).status).toBe('fulfilled');
  });

  it('a failed chunk resolves to null and warns', async () => {
    vi.resetModules();
    vi.doMock('@/i18n/catalogs/kk', () => {
      throw new Error('chunk load failed');
    });
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const fresh = await import('@/i18n/messages');
    expect(await fresh.loadBundles('kk', ['nav'], english)).toBeNull();
    expect(warn).toHaveBeenCalledOnce();
    vi.doUnmock('@/i18n/catalogs/kk');
    vi.restoreAllMocks();
  });
});
