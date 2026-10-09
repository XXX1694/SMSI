'use client';
import Link from 'next/link';
import { useRouter, useSearchParams } from 'next/navigation';
import { useState, type FormEvent } from 'react';
import { useAuth } from '@/components/auth-provider';
import { LegalLinks } from '@/components/legal/legal-links';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Field, Input } from '@/components/ui/input';
import { InlineError } from '@/components/states';
import { DEMO, DEMO_EMAIL, DEMO_PASSWORD } from '@/lib/demo/config';
import { useErrorText } from '@/hooks';
import { nodes } from '@/i18n/rich';
import { useTranslations } from '@/i18n/use-translations';

export function AuthForm({ mode }: { mode: 'login' | 'register' }) {
  const t = useTranslations('auth');
  const errorText = useErrorText();
  const { login, register } = useAuth();
  const router = useRouter();
  const params = useSearchParams();
  // The demo has a single built-in user, so its credentials are pre-filled.
  const [email, setEmail] = useState(DEMO && mode === 'login' ? DEMO_EMAIL : '');
  const [password, setPassword] = useState(DEMO && mode === 'login' ? DEMO_PASSWORD : '');
  const [name, setName] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [accepted, setAccepted] = useState(false);
  const [busy, setBusy] = useState(false);
  const isLogin = mode === 'login';

  async function submit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    if (!isLogin && password.length < 8) {
      setError(t('passwordShort'));
      return;
    }
    if (!isLogin && !accepted) {
      setError(t('acceptTerms'));
      return;
    }
    setBusy(true);
    try {
      if (isLogin) await login(email.trim(), password);
      else await register(email.trim(), password, name.trim(), accepted);
      const next = params.get('next');
      router.replace(next && next.startsWith('/') && !next.startsWith('//') ? next : '/dashboard');
    } catch (err) {
      setError(errorText(err));
      setBusy(false);
    }
  }

  return (
    <main className="flex min-h-screen items-center justify-center px-4">
      <div className="w-full max-w-sm">
        <h1 className="text-xl font-semibold tracking-tight">{isLogin ? t('signInTitle') : t('createTitle')}</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          {isLogin ? t('welcomeBack') : t('firstAccountNext')}
        </p>
        {DEMO ? (
          <p className="mt-3 rounded-md border bg-muted px-3 py-2 text-xs text-muted-foreground">
            {t('demoNote')}
          </p>
        ) : null}
        {isLogin && params.get('deleted') === '1' ? (
          <p role="status" className="mt-3 rounded-md border bg-muted px-3 py-2 text-sm">
            {t('deletedNotice')}
          </p>
        ) : null}
        <form onSubmit={submit} className="mt-8 space-y-4" noValidate>
          {!isLogin ? (
            <Field label={t('name')} htmlFor="name">
              <Input id="name" autoComplete="name" required value={name} onChange={(e) => setName(e.target.value)} />
            </Field>
          ) : null}
          <Field label={t('email')} htmlFor="email">
            <Input id="email" type="email" autoComplete="email" required value={email} onChange={(e) => setEmail(e.target.value)} />
          </Field>
          <Field label={t('password')} htmlFor="password" hint={isLogin ? undefined : t('passwordHint')}>
            <Input
              id="password"
              type="password"
              autoComplete={isLogin ? 'current-password' : 'new-password'}
              required
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          </Field>
          {!isLogin ? (
            <div className="flex items-start gap-2 text-sm">
              <Checkbox id="accept-terms" checked={accepted} onCheckedChange={(v) => setAccepted(v === true)} required />
              <label htmlFor="accept-terms" className="leading-snug">
                {nodes(
                  t.rich('agree', {
                    terms: (c) => (
                      <Link href="/terms" target="_blank" className="text-accent hover:underline">
                        {c}
                      </Link>
                    ),
                    privacy: (c) => (
                      <Link href="/privacy" target="_blank" className="text-accent hover:underline">
                        {c}
                      </Link>
                    ),
                  }),
                )}
              </label>
            </div>
          ) : null}
          {isLogin && !DEMO ? (
            <p className="-mt-2 text-right text-xs">
              <Link href="/forgot-password" className="text-accent hover:underline">
                {t('forgot')}
              </Link>
            </p>
          ) : null}
          {error ? (
            <InlineError>{error}</InlineError>
          ) : null}
          <Button type="submit" className="w-full" disabled={busy || !email || !password}>
            {busy ? (isLogin ? t('signingIn') : t('creating')) : isLogin ? t('signIn') : t('createAccount')}
          </Button>
        </form>
        <p className="mt-6 text-sm text-muted-foreground">
          {isLogin ? (
            <>
              {nodes(
                t.rich('noAccount', {
                  link: (c) => (
                    <Link href="/register" className="text-accent hover:underline">
                      {c}
                    </Link>
                  ),
                }),
              )}
            </>
          ) : (
            <>
              {nodes(
                t.rich('haveAccount', {
                  link: (c) => (
                    <Link href="/login" className="text-accent hover:underline">
                      {c}
                    </Link>
                  ),
                }),
              )}
            </>
          )}
        </p>
        <LegalLinks className="mt-6" />
      </div>
    </main>
  );
}
