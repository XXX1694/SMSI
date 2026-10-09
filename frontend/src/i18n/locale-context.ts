'use client';
import { createContext, useContext } from 'react';
import { FALLBACK_BUNDLES } from '@/i18n/fallback-bundles';
import type { AppLocale } from '@/i18n/locales';
import type { Catalog } from '@/i18n/pseudo';

/** What a `MessagesScope` tells the provider, so a locale switch can load its bundles before the UI changes. */
export interface ScopeRegistration {
  ids: readonly string[];
  /** The English bundles of the scope; the pseudo-locale is generated from them. */
  english: Catalog;
}

export interface LocaleSettings {
  locale: AppLocale;
  setLocale: (l: AppLocale) => void;
  /** What the language switcher lists. */
  available: AppLocale[];
  /** The bundles of the scopes above, in the active locale. A key missing from a translation reads English. */
  messages: Catalog;
  /** The same bundles in English: the last resort when a translated message fails to format. */
  english: Catalog;
  timeZone: string;
  /** Called by a scope on mount; returns the function that unregisters it. */
  register: (scope: ScopeRegistration) => () => void;
}

/**
 * Without a provider (a component rendered on its own in a test, an error boundary above the provider) the UI is English:
 * the same text the app had before it was localized.
 */
export const FALLBACK: LocaleSettings = {
  locale: 'en',
  setLocale: () => {},
  available: ['en'],
  messages: FALLBACK_BUNDLES as Catalog,
  english: FALLBACK_BUNDLES as Catalog,
  timeZone: 'UTC',
  register: () => () => {},
};

export const LocaleContext = createContext<LocaleSettings>(FALLBACK);

export function useLocaleSettings(): LocaleSettings {
  return useContext(LocaleContext);
}
