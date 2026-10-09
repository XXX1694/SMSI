import type { AppLocale } from '@/i18n/locales';

/**
 * The script faces that only their own locale needs (D-024). Each is a lazy CSS chunk of `@font-face` rules; the font files
 * themselves download only for characters a page shows (`unicode-range`), so a Japanese page fetches the slices for the
 * kana and kanji it uses, and English or Cyrillic pages fetch nothing from here. `globals.css` puts the family in the
 * stack through `--font-script` under `html:lang(...)`.
 */
const SCRIPT_FONTS: Partial<Record<AppLocale, () => Promise<unknown>>> = {
  ar: () => import('@fontsource-variable/noto-sans-arabic/wght.css'),
  ja: () => import('@fontsource-variable/noto-sans-jp/wght.css'),
  'zh-CN': () => import('@fontsource-variable/noto-sans-sc/wght.css'),
};

/** True when `locale` needs a script face beyond Onest. */
export function needsScriptFont(locale: AppLocale): boolean {
  return locale in SCRIPT_FONTS;
}

/**
 * Loads the locale's script face, if it has one. Never rejects: without the chunk (offline) the text falls back to the
 * system face for that script, which is readable, so the locale switch must not fail over a font.
 */
export async function loadScriptFont(locale: AppLocale): Promise<void> {
  try {
    await SCRIPT_FONTS[locale]?.();
  } catch {
    // The system face covers it; see above.
  }
}
