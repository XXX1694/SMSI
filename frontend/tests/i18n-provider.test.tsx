import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { PrefsProvider } from '@/components/prefs-provider';
import { LanguageSelect } from '@/i18n/language-select';
import { LocaleProvider, useLocaleSettings } from '@/i18n/locale-provider';
import { useFormat } from '@/i18n/use-format';
import { useTranslations } from '@/i18n/use-translations';

// A partial Arabic catalog: one translated key, the rest must fall back to English.
vi.mock('../messages/ar.json', () => ({ default: { nav: { dashboard: 'لوحة التحكم' } } }));

const ENABLED = ['en', 'ar'] as const;

function Probe() {
  const t = useTranslations('nav');
  const { locale } = useLocaleSettings();
  return (
    <p data-testid="probe">
      {locale}|{t('dashboard')}|{t('compose')}
    </p>
  );
}

function setup(ui = <Probe />, props: { userLocale?: string | null } = {}) {
  return render(
    <PrefsProvider>
      <LocaleProvider enabled={ENABLED} {...props}>
        {ui}
      </LocaleProvider>
    </PrefsProvider>,
  );
}

function languages(list: string[]) {
  vi.spyOn(window.navigator, 'languages', 'get').mockReturnValue(list);
}

beforeEach(() => {
  window.localStorage.clear();
  document.documentElement.lang = 'en';
  document.documentElement.dir = 'ltr';
  languages(['en-US']);
});
afterEach(() => vi.restoreAllMocks());

describe('LocaleProvider', () => {
  it('renders English with lang=en and dir=ltr by default', async () => {
    setup();
    expect(screen.getByTestId('probe')).toHaveTextContent('en|Dashboard|Compose');
    await waitFor(() => expect(document.documentElement.lang).toBe('en'));
    expect(document.documentElement.dir).toBe('ltr');
  });

  it('uses navigator.languages when nothing is stored, and sets dir=rtl for Arabic only', async () => {
    languages(['ar-EG']);
    setup();
    await waitFor(() => expect(screen.getByTestId('probe')).toHaveTextContent('ar|لوحة التحكم'));
    expect(document.documentElement.lang).toBe('ar');
    expect(document.documentElement.dir).toBe('rtl');
  });

  it('a missing key falls back to English', async () => {
    window.localStorage.setItem('socialos_locale', 'ar');
    setup();
    await waitFor(() => expect(screen.getByTestId('probe')).toHaveTextContent('ar|لوحة التحكم|Compose'));
  });

  it('localStorage beats navigator.languages; the user setting beats localStorage', async () => {
    languages(['ar']);
    window.localStorage.setItem('socialos_locale', 'en');
    setup();
    await waitFor(() => expect(document.documentElement.lang).toBe('en'));
    expect(screen.getByTestId('probe')).toHaveTextContent('en|Dashboard');
  });

  it('the user setting beats localStorage', async () => {
    window.localStorage.setItem('socialos_locale', 'en');
    setup(<Probe />, { userLocale: 'ar' });
    await waitFor(() => expect(screen.getByTestId('probe')).toHaveTextContent('ar|'));
  });

  it('ignores a stored locale that is not enabled', async () => {
    window.localStorage.setItem('socialos_locale', 'ru');
    setup();
    await waitFor(() => expect(document.documentElement.lang).toBe('en'));
    expect(screen.getByTestId('probe')).toHaveTextContent('en|Dashboard');
  });

  it('the pseudo-locale is available outside production builds', async () => {
    window.localStorage.setItem('socialos_locale', 'en-XA');
    setup();
    await waitFor(() => expect(screen.getByTestId('probe')).toHaveTextContent('en-XA|[Ď'));
    expect(document.documentElement.lang).toBe('en-XA');
    expect(document.documentElement.dir).toBe('ltr');
  });
});

describe('LanguageSelect', () => {
  it('lists only enabled locales (plus the pseudo-locale in dev), labels beta ones and persists the choice', async () => {
    setup(<LanguageSelect />);
    const select = screen.getByRole('combobox');
    const options = screen.getAllByRole('option').map((o) => o.textContent);
    expect(options).toEqual(['English', 'العربية (Beta translation)', 'Pseudo-locale (testing)']);
    await userEvent.selectOptions(select, 'ar');
    await waitFor(() => expect(document.documentElement.dir).toBe('rtl'));
    expect(window.localStorage.getItem('socialos_locale')).toBe('ar');
    await userEvent.selectOptions(screen.getByRole('combobox'), 'en');
    await waitFor(() => expect(document.documentElement.dir).toBe('ltr'));
    expect(window.localStorage.getItem('socialos_locale')).toBe('en');
  });

  it('the compact variant has an accessible name', () => {
    setup(<LanguageSelect compact />);
    expect(screen.getByRole('combobox', { name: 'Language' })).toBeInTheDocument();
  });

  it('survives a reload: the stored choice is applied on mount', async () => {
    const first = setup(<LanguageSelect />);
    await userEvent.selectOptions(screen.getByRole('combobox'), 'ar');
    await waitFor(() => expect(document.documentElement.dir).toBe('rtl'));
    first.unmount();
    document.documentElement.dir = 'ltr';
    setup(<Probe />);
    await waitFor(() => expect(document.documentElement.dir).toBe('rtl'));
  });
});

describe('useFormat', () => {
  it('uses the Settings timezone and the active locale', async () => {
    window.localStorage.setItem('socialos_tz', 'Asia/Almaty');
    function F() {
      const f = useFormat();
      return <p data-testid="f">{f.time('2026-10-09T23:30:00Z')}</p>;
    }
    setup(<F />);
    await waitFor(() => expect(screen.getByTestId('f')).toHaveTextContent('04:30'));
  });
});
