import { afterEach, describe, expect, it, vi } from 'vitest';
import { createTranslator } from '@/i18n/translate';

afterEach(() => vi.restoreAllMocks());

describe('the development guard for a namespace no scope loaded', () => {
  it('logs once, names the key and the missing namespace, and renders the key', () => {
    const logged = vi.spyOn(console, 'error').mockImplementation(() => {});
    const t = createTranslator({ locale: 'en', messages: { common: { loading: 'Loading…' } }, fallback: { common: { loading: 'Loading…' } } });
    expect(t('calendar.guardOnce' as never)).toBe('calendar.guardOnce');
    expect(t('calendar.guardOnce' as never)).toBe('calendar.guardOnce');
    expect(logged).toHaveBeenCalledTimes(1);
    expect(logged.mock.calls[0]?.[0]).toMatch(/MISSING_SCOPE: calendar\.guardOnce .*"calendar" messages are not loaded/);
  });

  it('calls a key missing from a loaded namespace a missing message, not a missing scope', () => {
    const logged = vi.spyOn(console, 'error').mockImplementation(() => {});
    const t = createTranslator({ locale: 'en', messages: { common: {} } });
    expect(t('common.guardMissingKey' as never)).toBe('common.guardMissingKey');
    expect(logged.mock.calls[0]?.[0]).toMatch(/^MISSING_MESSAGE: common\.guardMissingKey/);
  });

  it('stays silent in production', () => {
    const logged = vi.spyOn(console, 'error').mockImplementation(() => {});
    vi.stubEnv('NODE_ENV', 'production');
    try {
      const t = createTranslator({ locale: 'en', messages: {} });
      expect(t('posts.guardProduction' as never)).toBe('posts.guardProduction');
      expect(logged).not.toHaveBeenCalled();
    } finally {
      vi.unstubAllEnvs();
    }
  });
});
