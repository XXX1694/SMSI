'use client';
import Link from 'next/link';
import { useEffect, useRef, useState, type FormEvent } from 'react';
import { AuthShell } from '@/components/auth-shell';
import { Button } from '@/components/ui/button';
import { Field, Input } from '@/components/ui/input';
import { errorMessage } from '@/hooks';
import { ApiError, api } from '@/lib/api';
import { forgetHashToken, takeHashToken } from '@/lib/hash-token';

type State = 'form' | 'success' | 'invalid';

export function ResetPasswordView() {
  const [state, setState] = useState<State>('form');
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const token = useRef<string | null>(null);

  useEffect(() => {
    token.current = takeHashToken();
    if (!token.current) setState('invalid');
  }, []);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    if (password.length < 8 || password.length > 128) {
      setError('Password must be 8 to 128 characters.');
      return;
    }
    if (password !== confirm) {
      setError('The two passwords do not match.');
      return;
    }
    setBusy(true);
    try {
      await api.auth.resetPassword(token.current ?? '', password);
      forgetHashToken();
      token.current = null;
      setState('success');
    } catch (err) {
      // The password length is checked above, so a 400 here means the link is unknown, used or expired.
      if (err instanceof ApiError && err.status === 400) {
        forgetHashToken();
        token.current = null;
        setState('invalid');
      } else {
        setError(errorMessage(err));
      }
    } finally {
      setBusy(false);
    }
  }

  if (state === 'success') {
    return (
      <AuthShell title="Password updated" description="You were signed out everywhere. Sign in with your new password.">
        <Button asChild className="w-full">
          <Link href="/login">Sign in</Link>
        </Button>
      </AuthShell>
    );
  }
  if (state === 'invalid') {
    return (
      <AuthShell title="This link no longer works" description="It may have expired or already been used. Reset links work once and last 30 minutes.">
        <Button asChild className="w-full">
          <Link href="/forgot-password">Request a new link</Link>
        </Button>
      </AuthShell>
    );
  }
  return (
    <AuthShell title="Choose a new password" description="Every device will be signed out once you save it.">
      <form onSubmit={submit} className="space-y-4" noValidate>
        <Field label="New password" htmlFor="password" hint="At least 8 characters.">
          <Input id="password" type="password" autoComplete="new-password" required value={password} onChange={(e) => setPassword(e.target.value)} />
        </Field>
        <Field label="Repeat the new password" htmlFor="confirm">
          <Input id="confirm" type="password" autoComplete="new-password" required value={confirm} onChange={(e) => setConfirm(e.target.value)} />
        </Field>
        {error ? (
          <p role="alert" className="text-sm text-danger">
            {error}
          </p>
        ) : null}
        <Button type="submit" className="w-full" disabled={busy || !password || !confirm}>
          {busy ? 'Please wait…' : 'Save password'}
        </Button>
      </form>
    </AuthShell>
  );
}
