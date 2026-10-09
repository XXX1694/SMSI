import type { Metadata, Viewport } from 'next';
import { cookies } from 'next/headers';
import type { ReactNode } from 'react';
import { AuthProvider } from '@/components/auth-provider';
import { DemoBanner } from '@/components/demo-banner';
import { PrefsProvider } from '@/components/prefs-provider';
import { ToastProvider } from '@/components/toast';
import { BRAND_HEX } from '@/lib/brand';
import { LocaleProvider, type InitialLocale } from '@/i18n/locale-provider';
import { availableLocales, dirOf, isAvailable } from '@/i18n/locales';
import { loadMessages } from '@/i18n/messages';
import { localeScript } from '@/i18n/head-script';
import { LOCALE_STORAGE_KEY } from '@/i18n/resolve';
import { DEMO } from '@/lib/demo/config';
import en from '../../messages/en.json';
import './globals.css';

export const metadata: Metadata = {
  title: { default: 'Steerpost', template: '%s · Steerpost' },
  description: 'Compose, schedule and publish to your social accounts.',
};

export const viewport: Viewport = {
  width: 'device-width',
  initialScale: 1,
  themeColor: [
    { media: '(prefers-color-scheme: light)', color: BRAND_HEX.light.background },
    { media: '(prefers-color-scheme: dark)', color: BRAND_HEX.dark.background },
  ],
};

const THEME = `try{var t=localStorage.getItem('socialos_theme')||'system';var d=t==='dark'||(t==='system'&&matchMedia('(prefers-color-scheme: dark)').matches);document.documentElement.classList.toggle('dark',d)}catch(e){}`;

const PENDING_CSS = 'html[data-i18n-pending] body{visibility:hidden}';

/** Server build only: the locale cookie written by the switcher. The static demo has no request, so it never reads it. */
async function serverLocale(): Promise<InitialLocale | undefined> {
  if (DEMO) return undefined;
  const value = (await cookies()).get(LOCALE_STORAGE_KEY)?.value;
  if (!isAvailable(value) || value === 'en') return undefined;
  return { locale: value, messages: await loadMessages(value, en) };
}

export default async function RootLayout({ children }: { children: ReactNode }) {
  const initial = await serverLocale();
  const lang = initial?.locale ?? 'en';
  return (
    <html lang={lang} dir={dirOf(lang)} suppressHydrationWarning data-demo={DEMO ? '' : undefined}>
      <head>
        <style dangerouslySetInnerHTML={{ __html: PENDING_CSS }} />
        <script dangerouslySetInnerHTML={{ __html: THEME + localeScript(availableLocales()) }} />
      </head>
      <body>
        {process.env.NEXT_PUBLIC_DEMO === 'true' ? <DemoBanner /> : null}
        <PrefsProvider>
          <LocaleProvider initial={initial}>
            <ToastProvider>
              <AuthProvider>{children}</AuthProvider>
            </ToastProvider>
          </LocaleProvider>
        </PrefsProvider>
      </body>
    </html>
  );
}
