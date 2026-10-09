'use client';
import Link from 'next/link';
import { useSearchParams } from 'next/navigation';
import { AuthEmailForm } from '@/components/auth-email-form';
import { LegalLinks } from '@/components/legal/legal-links';
import { LoginErrorNotice } from '@/components/login-error-notice';
import { SocialSignIn } from '@/components/social-sign-in';
import { DEMO } from '@/lib/demo/config';
import { nodes } from '@/i18n/rich';
import { useTranslations } from '@/i18n/use-translations';
import { safeNext, withNext } from '@/lib/safe-next';

/** `/login` and `/register`: one screen. Provider buttons on top (only those the server offers), then the email form. */
export function AuthForm({ mode }: { mode: 'login' | 'register' }) {
  const t = useTranslations('auth');
  const params = useSearchParams();
  const isLogin = mode === 'login';
  const next = safeNext(params.get('next'));
  const error = isLogin ? params.get('error') : null;

  return (
    <main className="flex min-h-screen items-center justify-center px-4 py-8">
      <div className="w-full max-w-sm">
        <h1 className="text-xl font-semibold tracking-tight">{isLogin ? t('signInTitle') : t('createTitle')}</h1>
        <p className="mt-1 text-sm text-muted-foreground">{isLogin ? t('welcomeBack') : t('firstAccountNext')}</p>
        {DEMO ? <p className="mt-3 rounded-md border bg-muted px-3 py-2 text-xs text-muted-foreground">{t('demoNote')}</p> : null}
        {isLogin && params.get('deleted') === '1' ? (
          <p role="status" className="mt-3 rounded-md border bg-muted px-3 py-2 text-sm">
            {t('deletedNotice')}
          </p>
        ) : null}
        {error ? <LoginErrorNotice code={error} provider={params.get('provider')} /> : null}
        <SocialSignIn next={next} />
        <div className="mt-6">
          <AuthEmailForm mode={mode} next={next} />
        </div>
        <p className="mt-6 text-sm text-muted-foreground">
          {nodes(
            t.rich(isLogin ? 'noAccount' : 'haveAccount', {
              link: (c) => (
                <Link href={withNext(isLogin ? '/register' : '/login', next)} className="text-accent hover:underline">
                  {c}
                </Link>
              ),
            }),
          )}
        </p>
        <LegalLinks className="mt-6" />
      </div>
    </main>
  );
}
