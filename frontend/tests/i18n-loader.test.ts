import { describe, expect, it } from 'vitest';
import { BUNDLES } from '@/i18n/catalog';
import en from '@/i18n/en-all';
import { loadMessages } from '@/i18n/messages';
import type { Catalog } from '@/i18n/pseudo';

describe('locale loader assembles a catalog from its bundle files', () => {
  it('English is the static catalog and has every bundle', () => {
    expect(Object.keys(en)).toEqual([...BUNDLES]);
  });

  it('a translated locale has every bundle and its own text', async () => {
    const ru = await loadMessages('ru', en as Catalog);
    expect(Object.keys(ru).sort()).toEqual([...BUNDLES].sort());
    expect((ru.nav as Catalog).dashboard).not.toBe(en.nav.dashboard);
  });

  it('a locale with no bundle files is English', async () => {
    expect(await loadMessages('ar', en as Catalog)).toEqual(en);
  });
});
