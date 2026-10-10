'use client';
import { startTransition, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { usePrefs } from '@/components/prefs-provider';
import { LocaleContext, useLocaleSettings, type LocaleSettings, type ScopeRegistration } from '@/i18n/locale-context';
import { availableLocales, dirOf, ENABLED_LOCALES, type AppLocale, type Locale } from '@/i18n/locales';
import { loadBundles } from '@/i18n/messages';
import { LOCALE_STORAGE_KEY, resolveLocale } from '@/i18n/resolve';
import { loadScriptFont } from '@/i18n/script-fonts';
import { readStorage, writeStorage } from '@/lib/storage';

export { useLocaleSettings };

function applyDocument(locale: string): void {
  document.documentElement.lang = locale;
  document.documentElement.dir = dirOf(locale);
  document.documentElement.removeAttribute('data-i18n-pending');
}

/**
 * The active locale and how to change it: `activate` preloads every registered scope, then sets the locale in a
 * transition; lang, dir and the end of the head script's hiding are applied in the commit that shows the new text.
 */
function useActiveLocale() {
  const [locale, setActive] = useState<AppLocale>('en');
  const current = useRef<AppLocale>('en');
  const seq = useRef(0);
  const scopes = useRef(new Set<ScopeRegistration>());
  /** The locale whose document attributes are applied once it has committed. */
  const applyOnCommit = useRef<AppLocale | null>(null);

  const register = useCallback((scope: ScopeRegistration) => {
    scopes.current.add(scope);
    return () => void scopes.current.delete(scope);
  }, []);

  const activate = useCallback(async (next: AppLocale) => {
    const mine = ++seq.current;
    try {
      // Bundles of every mounted scope and the script face (ar, ja, zh-CN) load before the first frame in that locale.
      const loads = [...scopes.current].map((s) => loadBundles(next, s.ids, s.english, { retry: true }));
      const [results] = await Promise.all([Promise.all(loads), loadScriptFont(next)]);
      if (results.includes(null)) next = 'en'; // a chunk failed (offline, already logged): stay readable
    } catch {
      next = 'en';
    }
    if (mine !== seq.current) return; // a newer choice won
    if (next === current.current) return applyDocument(next);
    current.current = next;
    applyOnCommit.current = next;
    startTransition(() => setActive(next));
  }, []);

  // After the commit: lang, dir and the end of the head script's hiding happen together with the new text.
  useLayoutEffect(() => {
    if (applyOnCommit.current !== locale) return;
    applyOnCommit.current = null;
    applyDocument(locale);
  }, [locale]);
  return { locale, activate, register, current };
}

/**
 * Client-side locale state, no middleware and no locale routes (D-021), so it behaves the same in the server build and
 * the static demo. The provider holds the locale only; the messages come from the `MessagesScope`s below it, so render
 * `I18nRoot`, not this alone. Each scope imports its English statically; other locales are lazy chunks. Routes stay
 * static in both builds: the first render is English and the stored or detected locale swaps in after mount; a head
 * script hides the shell meanwhile (`data-i18n-pending`, at most 1.5 s) only when that locale is not English, so English
 * users see no change and others see no flash of English. A switch preloads every registered scope first, changes the
 * locale in a transition, and shows the shell again only after that commit.
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
  const { locale, activate, register, current } = useActiveLocale();

  useEffect(() => {
    const stored = readStorage(LOCALE_STORAGE_KEY);
    const next = resolveLocale({ user: userLocale, stored, languages: navigator.languages }, { enabled: stableEnabled });
    if (next !== current.current) void activate(next);
    else applyDocument(next);
  }, [activate, current, userLocale, stableEnabled]);

  const setLocale = useCallback(
    (l: AppLocale) => {
      writeStorage(LOCALE_STORAGE_KEY, l);
      // Not read by the app today (routes stay static); kept for future server use. Not a secret.
      document.cookie = `${LOCALE_STORAGE_KEY}=${l}; path=/; max-age=31536000; samesite=lax`;
      void activate(l);
    },
    [activate],
  );

  const settings = useMemo<LocaleSettings>(
    () => ({
      locale,
      setLocale,
      available: availableLocales({ enabled: stableEnabled }),
      messages: {},
      english: {},
      timeZone: timezone,
      register,
    }),
    [locale, setLocale, timezone, stableEnabled, register],
  );
  return <LocaleContext.Provider value={settings}>{children}</LocaleContext.Provider>;
}
