import type { Metadata, Viewport } from 'next';
import type { ReactNode } from 'react';
import { AuthProvider } from '@/components/auth-provider';
import { DemoBanner } from '@/components/demo-banner';
import { FocusOnNavigate } from '@/components/focus-on-navigate';
import { PrefsProvider } from '@/components/prefs-provider';
import { ToastProvider } from '@/components/toast';
import { BRAND_HEX } from '@/lib/brand';
import { LocaleProvider } from '@/i18n/locale-provider';
import { availableLocales } from '@/i18n/locales';
import { localeScript } from '@/i18n/head-script';
import { DEMO } from '@/lib/demo/config';
import './globals.css';

export const metadata: Metadata = {
  title: { default: 'Steerpost', template: '%s · Steerpost' },
  description: 'Compose, schedule and publish to your social accounts.',
};

export const viewport: Viewport = {
  width: 'device-width',
  initialScale: 1,
  // Lets env(safe-area-inset-*) report the notch and home-bar insets (toasts use the bottom one).
  viewportFit: 'cover',
  themeColor: [
    { media: '(prefers-color-scheme: light)', color: BRAND_HEX.light.canvas },
    { media: '(prefers-color-scheme: dark)', color: BRAND_HEX.dark.canvas },
  ],
};

const THEME = `try{var t=localStorage.getItem('socialos_theme')||'system';var d=t==='dark'||(t==='system'&&matchMedia('(prefers-color-scheme: dark)').matches);document.documentElement.classList.toggle('dark',d);if(localStorage.getItem('socialos_motion')==='off')document.documentElement.classList.add('motion-off')}catch(e){}`;

const PENDING_CSS = 'html[data-i18n-pending] body{visibility:hidden}';

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="en" suppressHydrationWarning data-demo={DEMO ? '' : undefined}>
      <head>
        <style dangerouslySetInnerHTML={{ __html: PENDING_CSS }} />
        <script dangerouslySetInnerHTML={{ __html: THEME + localeScript(availableLocales()) }} />
      </head>
      <body>
        <PrefsProvider>
          <LocaleProvider>
            {process.env.NEXT_PUBLIC_DEMO === 'true' ? <DemoBanner /> : null}
            <ToastProvider>
              <AuthProvider>{children}</AuthProvider>
              <FocusOnNavigate />
            </ToastProvider>
          </LocaleProvider>
        </PrefsProvider>
      </body>
    </html>
  );
}
