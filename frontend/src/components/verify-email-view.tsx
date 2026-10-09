'use client';
import Link from 'next/link';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useAuth } from '@/components/auth-provider';
import { AuthShell } from '@/components/auth-shell';
import { Button } from '@/components/ui/button';
import { InlineError } from '@/components/states';
import { errorMessage } from '@/hooks';
import { ApiError, api } from '@/lib/api';
import { forgetHashToken, takeHashToken } from '@/lib/hash-token';

type State = 'loading' | 'success' | 'invalid' | 'error';

export function VerifyEmailView() {
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
        setMessage(errorMessage(e));
        setState('error');
      }
    }
  }, [refresh]);

  useEffect(() => {
    if (started.current) return;
    started.current = true;
    token.current = takeHashToken();
    void run();
  }, [run]);

  if (state === 'loading') {
    return (
      <AuthShell title="Verifying your email">
        <p role="status" className="text-sm text-muted-foreground">
          One moment…
        </p>
      </AuthShell>
    );
  }
  if (state === 'success') {
    return (
      <AuthShell title="Email verified" description="Your email is verified. Every feature is unlocked.">
        <Button asChild className="w-full">
          <Link href={user ? '/dashboard' : '/login'}>{user ? 'Go to the dashboard' : 'Sign in'}</Link>
        </Button>
      </AuthShell>
    );
  }
  if (state === 'error') {
    return (
      <AuthShell title="Could not verify your email">
        <InlineError>{message}</InlineError>
        <Button className="mt-4 w-full" onClick={() => void run()}>
          Try again
        </Button>
      </AuthShell>
    );
  }
  return (
    <AuthShell title="This link no longer works" description="It may have expired or already been used. Links work once and last 48 hours.">
      <Button asChild className="w-full">
        <Link href={user ? '/settings' : '/login'}>{user ? 'Request a new link' : 'Sign in to request a new link'}</Link>
      </Button>
    </AuthShell>
  );
}
