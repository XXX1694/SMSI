'use client';
import { use, useContext, useEffect, useMemo, useRef, type ReactNode } from 'react';
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
  // Keyed by the namespace list, not by the object: an inline `bundles` literal must not re-register on every render.
  const idsKey = Object.keys(bundles).join(',');
  const ids = useMemo(() => (idsKey ? idsKey.split(',') : []), [idsKey]);
  const latest = useRef(english);
  useEffect(() => {
    latest.current = english;
  });

  // The provider preloads every registered scope before it changes the locale, so no key or English text flashes.
  useEffect(() => register({ ids, english: latest.current }), [register, ids]);

  const translated = locale === 'en' || ids.length === 0 ? null : use(loadBundles(locale, ids, english));
  // Keyed by the namespace list like `ids`, so an inline `bundles` literal does not rebuild the context on every render.
  const value = useMemo(
    () => ({
      ...parent,
      messages: { ...parent.messages, ...(translated ? mergeMessages(english, translated) : english) },
      english: { ...parent.english, ...english },
    }),
    // eslint-disable-next-line react-hooks/exhaustive-deps -- `english` is identified by `idsKey`
    [parent, idsKey, translated],
  );
  return <LocaleContext.Provider value={value}>{children}</LocaleContext.Provider>;
}
