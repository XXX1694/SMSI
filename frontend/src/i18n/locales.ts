/**
 * The locale registry (D-021). A locale appears in the switcher only when it is in `ENABLED_LOCALES`, which happens
 * when its catalog is complete and `npm run i18n:check` is green. `docs/copy/languages.md` has the rollout order.
 */
export const LOCALES = ['en', 'ru', 'es', 'pt-BR', 'de', 'fr', 'id', 'ja', 'zh-CN', 'kk', 'ar'] as const;
export type Locale = (typeof LOCALES)[number];

/** Locales whose catalog is complete. Add one here in the PR that finishes its catalog. */
export const ENABLED_LOCALES: readonly Locale[] = ['en', 'ru', 'es', 'pt-BR', 'de', 'fr', 'id', 'ja', 'zh-CN'];

/** Review state per locale (docs/copy/review/{locale}.md). Anything not `native-reviewed` shows "Beta translation". */
export type ReviewState = 'source' | 'machine-draft' | 'native-reviewed';
export const REVIEW: Record<Locale, ReviewState> = {
  en: 'source',
  ru: 'machine-draft',
  es: 'machine-draft',
  'pt-BR': 'machine-draft',
  de: 'machine-draft',
  fr: 'machine-draft',
  id: 'machine-draft',
  ja: 'machine-draft',
  'zh-CN': 'machine-draft',
  kk: 'machine-draft',
  ar: 'machine-draft',
};

/** Endonyms, no flags. */
export const ENDONYMS: Record<Locale, string> = {
  en: 'English',
  ru: 'Русский',
  es: 'Español',
  'pt-BR': 'Português (Brasil)',
  de: 'Deutsch',
  fr: 'Français',
  id: 'Bahasa Indonesia',
  ja: '日本語',
  'zh-CN': '简体中文',
  kk: 'Қазақша',
  ar: 'العربية',
};

/** The pseudo-locale: accented, about 35 % longer. Dev and demo only (see `pseudoEnabled`). */
export const PSEUDO_LOCALE = 'en-XA';

/** An app locale or the pseudo-locale. */
export type AppLocale = Locale | typeof PSEUDO_LOCALE;

export function isLocale(value: unknown): value is Locale {
  return typeof value === 'string' && (LOCALES as readonly string[]).includes(value);
}

/** Pseudo-locale on: dev server, the static demo, or an explicit opt-in. Never in a production build. */
export function pseudoEnabled(): boolean {
  return (
    process.env.NODE_ENV !== 'production' ||
    process.env.NEXT_PUBLIC_DEMO === 'true' ||
    process.env.NEXT_PUBLIC_I18N_PSEUDO === '1'
  );
}

/** Locales a user may pick right now. */
export function availableLocales(opts: { enabled?: readonly Locale[]; pseudo?: boolean } = {}): AppLocale[] {
  const enabled = opts.enabled ?? ENABLED_LOCALES;
  const list: AppLocale[] = LOCALES.filter((l) => enabled.includes(l));
  if (opts.pseudo ?? pseudoEnabled()) list.push(PSEUDO_LOCALE);
  return list;
}

export function isAvailable(value: unknown, opts: { enabled?: readonly Locale[]; pseudo?: boolean } = {}): value is AppLocale {
  return typeof value === 'string' && (availableLocales(opts) as string[]).includes(value);
}

export function isBeta(locale: AppLocale): boolean {
  return locale !== PSEUDO_LOCALE && REVIEW[locale] === 'machine-draft';
}

/** `dir` of a locale: right-to-left only for Arabic. */
export function dirOf(locale: string): 'ltr' | 'rtl' {
  return locale === 'ar' || locale.startsWith('ar-') ? 'rtl' : 'ltr';
}

/** The BCP 47 tag handed to `Intl`. `en` keeps the day-first output the app has always had. */
export function formatTag(locale: string): string {
  if (locale === 'en' || locale === PSEUDO_LOCALE) return 'en-GB';
  if (locale === 'ar') return 'ar-u-nu-latn-ca-gregory';
  return locale;
}
