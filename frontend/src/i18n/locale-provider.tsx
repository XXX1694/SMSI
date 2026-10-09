'use client';
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { usePrefs } from '@/components/prefs-provider';
import { availableLocales, dirOf, ENABLED_LOCALES, type AppLocale, type Locale } from '@/i18n/locales';
import { loadMessages } from '@/i18n/messages';
import type { Catalog } from '@/i18n/pseudo';
import { LOCALE_STORAGE_KEY, resolveLocale } from '@/i18n/resolve';
import { loadScriptFont } from '@/i18n/script-fonts';
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

/**
 * Without a provider (a component rendered on its own in a test, an error boundary above the provider) the UI is English:
 * the same text the app had before it was localized.
 */
const FALLBACK: LocaleSettings = { locale: 'en', setLocale: () => {}, available: ['en'], messages: en, timeZone: 'UTC' };

const LocaleContext = createContext<LocaleSettings>(FALLBACK);

interface LocaleState {
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
 * chunks merged over it. Routes stay static in both builds: the first render is English and the stored or detected locale
 * swaps in after mount; a head script hides the shell meanwhile (`data-i18n-pending`, at most 1.5 s) only when that locale
 * is not English, so English users see no change and others see no flash of English.
 */
export function LocaleProvider({
  children,
  userLocale = null,
  enabled = ENABLED_LOCALES,
}: {
  children: ReactNode;
  /** `users.locale` once the API returns it; resolution order is user setting, localStorage, navigator, en. */
  userLocale?: string | null;
  /** Which locales may be chosen; tests override it. */
  enabled?: readonly Locale[];
}) {
  const { timezone } = usePrefs();
  const enabledKey = enabled.join(',');
  // eslint-disable-next-line react-hooks/exhaustive-deps -- keyed by content, so an inline array does not re-run the effect
  const stableEnabled = useMemo(() => enabled, [enabledKey]);
  const [state, setState] = useState<LocaleState>({ locale: 'en', messages: en });
  const current = useRef(state.locale);
  const seq = useRef(0);

  const activate = useCallback(async (next: AppLocale) => {
    const mine = ++seq.current;
    let messages: Catalog = en;
    try {
      // The script face (ar, ja, zh-CN) loads alongside the catalog, so the first frame in that locale has its font rules.
      [messages] = await Promise.all([loadMessages(next, en), loadScriptFont(next)]);
    } catch {
      next = 'en'; // chunk failed to load (offline): stay readable
    }
    if (mine !== seq.current) return; // a newer choice won
    applyDocument(next);
    current.current = next;
    setState({ locale: next, messages });
  }, []);

  useEffect(() => {
    const stored = readStorage(LOCALE_STORAGE_KEY);
    const next = resolveLocale({ user: userLocale, stored, languages: navigator.languages }, { enabled: stableEnabled });
    if (next !== current.current) void activate(next);
    else applyDocument(next);
  }, [activate, userLocale, stableEnabled]);

  const setLocale = useCallback(
    (l: AppLocale) => {
      writeStorage(LOCALE_STORAGE_KEY, l);
      // Not read by the app today (routes stay static); kept for future server use. Not a secret.
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
  return useContext(LocaleContext);
}
