'use client';
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { usePrefs } from '@/components/prefs-provider';
import { availableLocales, dirOf, ENABLED_LOCALES, type AppLocale, type Locale } from '@/i18n/locales';
import { loadMessages } from '@/i18n/messages';
import type { Catalog } from '@/i18n/pseudo';
import { LOCALE_STORAGE_KEY, resolveLocale } from '@/i18n/resolve';
import { readStorage, writeStorage } from '@/lib/storage';
import en from '../../messages/en.json';

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

/** What the server already rendered: set from the locale cookie by the root layout (server build only). */
export interface InitialLocale {
  locale: AppLocale;
  messages: Catalog;
}

function applyDocument(locale: string): void {
  document.documentElement.lang = locale;
  document.documentElement.dir = dirOf(locale);
  document.documentElement.removeAttribute('data-i18n-pending');
}

/**
 * Client-side locale state, no middleware and no locale routes (D-021), so it behaves the same in the server build and
 * the static demo. English is a static import (a cached JS chunk, not part of every document); other catalogs are lazy
 * chunks merged over it. In the server build the layout reads the locale cookie, so a non-English first paint is already
 * right (`initial`). In the demo the stored locale swaps in after mount, with the shell hidden meanwhile (head script).
 */
export function LocaleProvider({
  children,
  initial,
  userLocale = null,
  enabled = ENABLED_LOCALES,
}: {
  children: ReactNode;
  initial?: InitialLocale;
  /** `users.locale` once the API returns it; resolution order is user setting, localStorage, navigator, en. */
  userLocale?: string | null;
  /** Which locales may be chosen; tests override it. */
  enabled?: readonly Locale[];
}) {
  const { timezone } = usePrefs();
  const enabledKey = enabled.join(',');
  // eslint-disable-next-line react-hooks/exhaustive-deps -- keyed by content, so an inline array does not re-run the effect
  const stableEnabled = useMemo(() => enabled, [enabledKey]);
  const [state, setState] = useState<InitialLocale>(initial ?? { locale: 'en', messages: en });
  const current = useRef(state.locale);
  const seq = useRef(0);

  const activate = useCallback(async (next: AppLocale) => {
    const mine = ++seq.current;
    let messages: Catalog = en;
    try {
      messages = await loadMessages(next, en);
    } catch {
      next = 'en'; // chunk failed to load (offline): stay readable
    }
    if (mine !== seq.current) return; // a newer choice won
    applyDocument(next);
    current.current = next;
    setState({ locale: next, messages });
  }, []);

  useEffect(() => {
    const stored = readStorage(LOCALE_STORAGE_KEY) ?? initial?.locale ?? null;
    const next = resolveLocale({ user: userLocale, stored, languages: navigator.languages }, { enabled: stableEnabled });
    if (next !== current.current) void activate(next);
    else applyDocument(next);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- `initial` only seeds the first resolution
  }, [activate, userLocale, stableEnabled]);

  const setLocale = useCallback(
    (l: AppLocale) => {
      writeStorage(LOCALE_STORAGE_KEY, l);
      // The server build reads this cookie so the next first paint is already in the right language. Not a secret.
      document.cookie = `${LOCALE_STORAGE_KEY}=${l}; path=/; max-age=31536000; samesite=lax`;
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
