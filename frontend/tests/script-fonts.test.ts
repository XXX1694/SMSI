import { describe, expect, it, vi } from 'vitest';

describe('script fonts', () => {
  it('needs a script face only for Arabic, Japanese and Simplified Chinese', async () => {
    const { needsScriptFont } = await import('../src/i18n/script-fonts');
    expect(['ar', 'ja', 'zh-CN'].every((l) => needsScriptFont(l as 'ar'))).toBe(true);
    for (const l of ['en', 'ru', 'kk', 'es', 'pt-BR', 'de', 'fr', 'id', 'en-XA'] as const) expect(needsScriptFont(l)).toBe(false);
  });

  it('loads the face for its locale and resolves without one for others', async () => {
    const { loadScriptFont } = await import('../src/i18n/script-fonts');
    await expect(loadScriptFont('ja')).resolves.toBeUndefined();
    await expect(loadScriptFont('en')).resolves.toBeUndefined();
  });

  it('does not fail the locale switch when the font chunk cannot load', async () => {
    vi.resetModules();
    vi.doMock('@fontsource-variable/noto-sans-arabic/wght.css', () => {
      throw new Error('offline');
    });
    const { loadScriptFont } = await import('../src/i18n/script-fonts');
    await expect(loadScriptFont('ar')).resolves.toBeUndefined();
    vi.doUnmock('@fontsource-variable/noto-sans-arabic/wght.css');
  });
});
