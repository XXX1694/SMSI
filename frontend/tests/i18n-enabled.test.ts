import { describe, expect, it } from 'vitest';
import { availableLocales, ENABLED_LOCALES, ENDONYMS, isBeta } from '@/i18n/locales';

describe('enabled locales (D-021)', () => {
  it('offers English and the eight reviewed-by-machine locales, in switcher order', () => {
    expect([...ENABLED_LOCALES]).toEqual(['en', 'ru', 'es', 'pt-BR', 'de', 'fr', 'id', 'ja', 'zh-CN']);
  });

  it('keeps kk hidden until a native review and ar until the RTL work lands', () => {
    expect(ENABLED_LOCALES).not.toContain('kk');
    expect(ENABLED_LOCALES).not.toContain('ar');
  });

  it('lists every enabled locale by its endonym and labels all but English as beta', () => {
    const list = availableLocales({ pseudo: false });
    expect(list.map((l) => ENDONYMS[l as keyof typeof ENDONYMS])).toEqual([
      'English', 'Русский', 'Español', 'Português (Brasil)', 'Deutsch', 'Français', 'Bahasa Indonesia', '日本語', '简体中文',
    ]);
    expect(list.filter((l) => isBeta(l))).toEqual(list.slice(1));
  });
});
