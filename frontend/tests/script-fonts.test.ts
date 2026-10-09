import { describe, expect, it, vi } from 'vitest';

describe('script fonts', () => {
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
