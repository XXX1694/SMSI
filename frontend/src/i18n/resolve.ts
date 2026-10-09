import { ENABLED_LOCALES, isAvailable, type AppLocale, type Locale } from '@/i18n/locales';
import { matchLocale } from '@/i18n/match';

export const LOCALE_STORAGE_KEY = 'socialos_locale';

export interface ResolveInput {
  /** `users.locale` once the server stores it (later PR); null until then. */
  user?: string | null;
  stored?: string | null;
  languages?: readonly string[];
}

/** user setting → localStorage → navigator.languages → en. Every step is checked against the available locales. */
export function resolveLocale(input: ResolveInput, opts: { enabled?: readonly Locale[]; pseudo?: boolean } = {}): AppLocale {
  if (isAvailable(input.user, opts)) return input.user;
  if (isAvailable(input.stored, opts)) return input.stored;
  return matchLocale(input.languages ?? [], opts.enabled ?? ENABLED_LOCALES);
}
