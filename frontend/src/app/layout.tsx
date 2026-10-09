import type { Metadata, Viewport } from 'next';
import type { ReactNode } from 'react';
import { AuthProvider } from '@/components/auth-provider';
import { DemoBanner } from '@/components/demo-banner';
import { PrefsProvider } from '@/components/prefs-provider';
import { ToastProvider } from '@/components/toast';
import { DEMO } from '@/lib/demo/config';
import './globals.css';

export const metadata: Metadata = {
  title: { default: 'Steerpost', template: '%s · Steerpost' },
  description: 'Compose, schedule and publish to your social accounts.',
};

export const viewport: Viewport = { width: 'device-width', initialScale: 1 };

const themeScript = `try{var t=localStorage.getItem('socialos_theme')||'system';var d=t==='dark'||(t==='system'&&matchMedia('(prefers-color-scheme: dark)').matches);document.documentElement.classList.toggle('dark',d)}catch(e){}`;

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="en" suppressHydrationWarning data-demo={DEMO ? '' : undefined}>
      <head>
        <script dangerouslySetInnerHTML={{ __html: themeScript }} />
      </head>
      <body>
        {process.env.NEXT_PUBLIC_DEMO === 'true' ? <DemoBanner /> : null}
        <PrefsProvider>
          <ToastProvider>
            <AuthProvider>{children}</AuthProvider>
          </ToastProvider>
        </PrefsProvider>
      </body>
    </html>
  );
}
