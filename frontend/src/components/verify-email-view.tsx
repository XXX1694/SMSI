'use client';
import Link from 'next/link';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useAuth } from '@/components/auth-provider';
import { AuthShell } from '@/components/auth-shell';
import { Button } from '@/components/ui/button';
import { InlineError } from '@/components/states';
import { useErrorText } from '@/hooks';
import { useTranslations } from '@/i18n/use-translations';
import { ApiError, api } from '@/lib/api';
import { forgetHashToken, takeHashToken } from '@/lib/hash-token';

type State = 'loading' | 'success' | 'invalid' | 'error';

export function VerifyEmailView() {
  const t = useTranslations('auth.verifyEmail');
  const ta = useTranslations('auth');
  const tc = useTranslations('common');
  const errorText = useErrorText();
  const { user, refresh } = useAuth();
  const [state, setState] = useState<State>('loading');
  const [message, setMessage] = useState('');
  const started = useRef(false);
  const token = useRef<string | null>(null);

  const run = useCallback(async () => {
    setState('loading');
    if (!token.current) {
      setState('invalid');
      return;
    }
    try {
      await api.auth.verifyEmail(token.current);
      forgetHashToken();
      token.current = null;
      setState('success');
      await refresh();
    } catch (e) {
      // 400 = unknown, already used or expired. Anything else (network, 5xx) may work on a retry.
      if (e instanceof ApiError && e.status === 400) {
        forgetHashToken();
        token.current = null;
        setState('invalid');
      } else {
        setMessage(errorText(e));
        setState('error');
      }
    }
  }, [refresh, errorText]);

  useEffect(() => {
    if (started.current) return;
    started.current = true;
    token.current = takeHashToken();
    void run();
  }, [run]);

  if (state === 'loading') {
    return (
      <AuthShell title={t('title')}>
        <p role="status" className="text-sm text-muted-foreground">
          {t('moment')}
        </p>
      </AuthShell>
    );
  }
  if (state === 'success') {
    return (
      <AuthShell title={t('doneTitle')} description={t('doneBody')}>
        <Button asChild className="w-full">
          <Link href={user ? '/dashboard' : '/login'}>{user ? t('toDashboard') : ta('signIn')}</Link>
        </Button>
      </AuthShell>
    );
  }
  if (state === 'error') {
    return (
      <AuthShell title={t('failTitle')}>
        <InlineError>{message}</InlineError>
        <Button className="mt-4 w-full" onClick={() => void run()}>
          {tc('tryAgain')}
        </Button>
      </AuthShell>
    );
  }
  return (
    <AuthShell title={ta('linkInvalidTitle')} description={t('invalidBody')}>
      <Button asChild className="w-full">
        <Link href={user ? '/settings' : '/login'}>{user ? ta('requestNewLink') : t('signInToRequest')}</Link>
      </Button>
    </AuthShell>
  );
}
