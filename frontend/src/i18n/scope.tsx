'use client';
import { use, useContext, useEffect, useMemo, type ReactNode } from 'react';
import type { Messages } from '@/i18n/catalog';
import { LocaleContext } from '@/i18n/locale-context';
import { loadBundles, mergeMessages } from '@/i18n/messages';
import type { Catalog } from '@/i18n/pseudo';

/**
 * Gives the subtree the message bundles it needs. `bundles` are the English JSON files, imported statically by the
 * caller, so English ships with the code that uses it. In `en` the scope renders at once. In another locale it reads the
 * translated bundles with `use()` (suspending until the locale's chunk arrives, unless a language switch already
 * preloaded them) and lays them over the English, so a missing key reads English. A chunk that fails to load is logged
 * and the subtree stays English. Bundles merge over the scopes above by namespace: an inner scope wins.
 */
export function MessagesScope({ bundles, children }: { bundles: Partial<Messages>; children: ReactNode }) {
  const parent = useContext(LocaleContext);
  const { locale, register } = parent;
  const english = bundles as Catalog;
  const ids = useMemo(() => Object.keys(bundles), [bundles]);

  // The provider preloads every registered scope before it changes the locale, so no key or English text flashes.
  useEffect(() => register({ ids, english }), [register, ids, english]);

  const translated = locale === 'en' || ids.length === 0 ? null : use(loadBundles(locale, ids, english));
  const value = useMemo(
    () => ({
      ...parent,
      messages: { ...parent.messages, ...(translated ? mergeMessages(english, translated) : english) },
      english: { ...parent.english, ...english },
    }),
    [parent, english, translated],
  );
  return <LocaleContext.Provider value={value}>{children}</LocaleContext.Provider>;
}
