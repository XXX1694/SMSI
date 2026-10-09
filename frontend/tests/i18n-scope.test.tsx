import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Suspense, useLayoutEffect, type ReactNode } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { PrefsProvider } from '@/components/prefs-provider';
import en from '@/i18n/en-all';
import { LanguageSelect } from '@/i18n/language-select';
import { FALLBACK, LocaleContext } from '@/i18n/locale-context';
import { LocaleProvider } from '@/i18n/locale-provider';
import { MessagesScope } from '@/i18n/scope';
import { useTranslations } from '@/i18n/use-translations';
import ruNav from '../messages/ru/nav.json';

// Partial catalogs: one translated key each, the rest must read English; kk's chunk never loads.
vi.mock('@/i18n/catalogs/ja', () => ({ default: { nav: { dashboard: 'JA-DASH' } } }));
vi.mock('@/i18n/catalogs/kk', () => {
  throw new Error('chunk load failed');
});

// The scope under test supplies its own English so it is the only source of text (the vitest fallback is full English).
const NAV = { nav: { ...en.nav, dashboard: 'Scope dashboard' } };

function Probe({ log, events }: { log?: string[]; events?: string[] }) {
  const t = useTranslations('nav');
  const text = `${t('dashboard')}|${t('compose')}`;
  log?.push(text);
  useLayoutEffect(() => {
    events?.push(`committed ${text}`);
  });
  return <p data-testid="probe">{text}</p>;
}

/** A context standing in for the provider, in a given locale, with no preloading. */
const fallbackRenders: string[] = [];
function Fallback() {
  fallbackRenders.push('loading');
  return <p>loading</p>;
}

async function inLocale(locale: 'en' | 'ru' | 'ja' | 'kk', ui: ReactNode) {
  // Awaited: rendering suspends inside act() until the locale chunk has loaded.
  await act(async () => {
    render(
      <LocaleContext.Provider value={{ ...FALLBACK, locale }}>
        <Suspense fallback={<Fallback />}>{ui}</Suspense>
      </LocaleContext.Provider>,
    );
  });
}

beforeEach(() => {
  window.localStorage.clear();
  fallbackRenders.length = 0;
});
afterEach(() => vi.restoreAllMocks());

describe('MessagesScope', () => {
  it('renders English synchronously', () => {
    render(
      <LocaleContext.Provider value={FALLBACK}>
        <Suspense fallback={<Fallback />}>
          <MessagesScope bundles={NAV}>
            <Probe />
          </MessagesScope>
        </Suspense>
      </LocaleContext.Provider>,
    );
    expect(screen.getByTestId('probe')).toHaveTextContent('Scope dashboard|Compose');
    expect(fallbackRenders).toEqual([]);
  });

  it('suspends in another locale, then renders the translation', async () => {
    await inLocale(
      'ru',
      <MessagesScope bundles={NAV}>
        <Probe />
      </MessagesScope>,
    );
    expect(fallbackRenders.length).toBeGreaterThan(0); // it suspended until the chunk arrived
    expect(await screen.findByTestId('probe')).toHaveTextContent(`${ruNav.dashboard}|${ruNav.compose}`);
  });

  it('a key missing from the translation reads English', async () => {
    await inLocale(
      'ja',
      <MessagesScope bundles={NAV}>
        <Probe />
      </MessagesScope>,
    );
    expect(await screen.findByTestId('probe')).toHaveTextContent('JA-DASH|Compose');
  });

  it('a chunk that fails to load logs a warning and the subtree stays English', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    await inLocale(
      'kk',
      <MessagesScope bundles={NAV}>
        <Probe />
      </MessagesScope>,
    );
    expect(await screen.findByTestId('probe')).toHaveTextContent('Scope dashboard|Compose');
    expect(warn).toHaveBeenCalledWith(expect.stringContaining('kk'), expect.anything());
  });
});

describe('switching the language', () => {
  it('never shows a key or half-translated text, and shows the shell only after the commit', async () => {
    const html = document.documentElement;
    const events: string[] = [];
    const observer = new MutationObserver(() => {
      if (!html.hasAttribute('data-i18n-pending')) events.push('shell shown');
    });
    observer.observe(html, { attributes: true, attributeFilter: ['data-i18n-pending'] });
    const texts: string[] = [];
    render(
      <PrefsProvider>
        <LocaleProvider enabled={['en', 'ru']}>
          <MessagesScope bundles={NAV}>
            <LanguageSelect />
            <Probe log={texts} events={events} />
          </MessagesScope>
        </LocaleProvider>
      </PrefsProvider>,
    );
    await act(async () => {});
    html.setAttribute('data-i18n-pending', ''); // what the head script leaves while a stored locale loads
    await userEvent.selectOptions(screen.getByRole('combobox'), 'ru');
    await waitFor(() => expect(screen.getByTestId('probe')).toHaveTextContent(ruNav.dashboard));
    await act(async () => {});
    await userEvent.selectOptions(screen.getByRole('combobox'), 'en');
    await waitFor(() => expect(screen.getByTestId('probe')).toHaveTextContent('Scope dashboard|Compose'));
    observer.disconnect();

    const english = 'Scope dashboard|Compose';
    const russian = `${ruNav.dashboard}|${ruNav.compose}`;
    expect(texts.every((x) => x === english || x === russian)).toBe(true);
    expect(texts.some((x) => x.includes('nav.'))).toBe(false);
    const ruCommit = events.indexOf(`committed ${russian}`);
    expect(ruCommit).toBeGreaterThan(-1);
    expect(events.indexOf('shell shown')).toBeGreaterThan(ruCommit);
  });
});
