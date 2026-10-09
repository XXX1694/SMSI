'use client';
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { usePrefs } from '@/components/prefs-provider';
import { availableLocales, dirOf, ENABLED_LOCALES, type AppLocale, type Locale } from '@/i18n/locales';
import { loadMessages } from '@/i18n/messages';
import type { Catalog } from '@/i18n/pseudo';
import { LOCALE_STORAGE_KEY, resolveLocale } from '@/i18n/resolve';
import { readStorage, writeStorage } from '@/lib/storage';

interface LocaleSettings {
  locale: AppLocale;
  setLocale: (l: AppLocale) => void;
  /** What the language switcher lists. */
  available: AppLocale[];
  /** English merged under the active locale, so a missing key falls back to English. */
  messages: Catalog;
  timeZone: string;
}

const LocaleContext = createContext<LocaleSettings | null>(null);

function applyDocument(locale: string): void {
  document.documentElement.lang = locale;
  document.documentElement.dir = dirOf(locale);
}

/**
 * Client-side locale state, no middleware and no locale routes (D-021), so it behaves the same in the server build and
 * the static demo. English is passed in as a prop from the server layout; other catalogs are lazy chunks merged over it.
 * The first render is English (what the server rendered); the stored or detected locale swaps in after mount.
 */
export function LocaleProvider({
  children,
  enMessages,
  userLocale = null,
  enabled = ENABLED_LOCALES,
}: {
  children: ReactNode;
  enMessages: Catalog;
  /** `users.locale` once the API returns it; resolution order is user setting, localStorage, navigator, en. */
  userLocale?: string | null;
  /** Which locales may be chosen; tests override it. */
  enabled?: readonly Locale[];
}) {
  const { timezone } = usePrefs();
  const enabledKey = enabled.join(',');
  // eslint-disable-next-line react-hooks/exhaustive-deps -- keyed by content, so an inline array does not re-run the effect
  const stableEnabled = useMemo(() => enabled, [enabledKey]);
  const [state, setState] = useState<{ locale: AppLocale; messages: Catalog }>({ locale: 'en', messages: enMessages });
  const seq = useRef(0);

  const activate = useCallback(
    async (next: AppLocale) => {
      const mine = ++seq.current;
      let messages = enMessages;
      try {
        messages = await loadMessages(next, enMessages);
      } catch {
        next = 'en'; // chunk failed to load (offline): stay readable
      }
      if (mine !== seq.current) return; // a newer choice won
      applyDocument(next);
      setState({ locale: next, messages });
    },
    [enMessages],
  );

  useEffect(() => {
    const next = resolveLocale({ user: userLocale, stored: readStorage(LOCALE_STORAGE_KEY), languages: navigator.languages }, { enabled: stableEnabled });
    if (next !== 'en') void activate(next);
    else applyDocument('en');
  }, [activate, userLocale, stableEnabled]);

  const setLocale = useCallback(
    (l: AppLocale) => {
      writeStorage(LOCALE_STORAGE_KEY, l);
      void activate(l);
    },
    [activate],
  );

  const settings = useMemo(
    () => ({ locale: state.locale, setLocale, available: availableLocales({ enabled: stableEnabled }), messages: state.messages, timeZone: timezone }),
    [state, setLocale, timezone, stableEnabled],
  );
  return <LocaleContext.Provider value={settings}>{children}</LocaleContext.Provider>;
}

export function useLocaleSettings(): LocaleSettings {
  const ctx = useContext(LocaleContext);
  if (!ctx) throw new Error('useLocaleSettings must be used within LocaleProvider');
  return ctx;
}
