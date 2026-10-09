/**
 * The landing-page locales (D-021, docs/copy/languages.md). Only the landing is localized; docs and the demo stay English.
 *
 *   code      the catalog name (site/i18n/landing.<code>.json) and the app's locale code
 *   slug      the URL folder under the site base ('' = the site root). Lower case, GitHub Pages is case-sensitive.
 *   hreflang  the tag used in <link rel="alternate">; og is Open Graph's locale
 *   name      the endonym shown in the switcher (no flags)
 *   review    mirrors REVIEW in frontend/src/i18n/locales.ts: 'source' | 'machine-draft' | 'native-reviewed'. Anything but
 *             'native-reviewed' (and 'source', English) shows the "Beta translation" note; record it in docs/copy/review/<code>.md
 *   hidden    built and noindex, but not linked, not in the switcher, not in hreflang or the sitemap
 *   cjk       no spaces between words: headline words are written as phrases and joined without a space
 */
export const LOCALES = [
  { code: 'en', slug: '', lang: 'en', hreflang: 'en', og: 'en_US', dir: 'ltr', name: 'English', short: 'EN', review: 'source' },
  { code: 'ru', slug: 'ru', lang: 'ru', hreflang: 'ru', og: 'ru_RU', dir: 'ltr', name: 'Русский', short: 'RU', review: 'machine-draft' },
  { code: 'es', slug: 'es', lang: 'es', hreflang: 'es', og: 'es_ES', dir: 'ltr', name: 'Español', short: 'ES', review: 'machine-draft' },
  { code: 'pt-BR', slug: 'pt-br', lang: 'pt-BR', hreflang: 'pt-BR', og: 'pt_BR', dir: 'ltr', name: 'Português (Brasil)', short: 'PT', review: 'machine-draft' },
  { code: 'de', slug: 'de', lang: 'de', hreflang: 'de', og: 'de_DE', dir: 'ltr', name: 'Deutsch', short: 'DE', review: 'machine-draft' },
  { code: 'fr', slug: 'fr', lang: 'fr', hreflang: 'fr', og: 'fr_FR', dir: 'ltr', name: 'Français', short: 'FR', review: 'machine-draft' },
  { code: 'id', slug: 'id', lang: 'id', hreflang: 'id', og: 'id_ID', dir: 'ltr', name: 'Bahasa Indonesia', short: 'ID', review: 'machine-draft' },
  { code: 'ja', slug: 'ja', lang: 'ja', hreflang: 'ja', og: 'ja_JP', dir: 'ltr', name: '日本語', short: 'JA', review: 'machine-draft', cjk: true },
  { code: 'zh-CN', slug: 'zh-cn', lang: 'zh-CN', hreflang: 'zh-CN', og: 'zh_CN', dir: 'ltr', name: '简体中文', short: '中文', review: 'machine-draft', cjk: true },
  { code: 'ar', slug: 'ar', lang: 'ar', hreflang: 'ar', og: 'ar_AR', dir: 'rtl', name: 'العربية', short: 'ع', review: 'machine-draft' },
  { code: 'kk', slug: 'kk', lang: 'kk', hreflang: 'kk', og: 'kk_KZ', dir: 'ltr', name: 'Қазақша', short: 'KK', review: 'machine-draft', hidden: true },
];

