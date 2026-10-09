'use client';
import Link from 'next/link';
import { useState, type FormEvent } from 'react';
import { AuthShell } from '@/components/auth-shell';
import { InlineError, Notice } from '@/components/states';
import { Button } from '@/components/ui/button';
import { Field, Input } from '@/components/ui/input';
import { errorMessage } from '@/hooks';
import { api } from '@/lib/api';

export function ForgotPasswordView() {
  const [email, setEmail] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [sent, setSent] = useState<{ delivery: 'log' | 'smtp' } | null>(null);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    setBusy(true);
    try {
      setSent(await api.auth.forgotPassword(email.trim()));
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  if (sent) {
    return (
      <AuthShell title="Check your inbox" description="If an account exists for that address, Steerpost sent a link to reset the password. It works once and lasts 30 minutes.">
        {sent.delivery === 'log' ? (
          <Notice tone="warning">Email delivery is not set up on this server, so no message will arrive. Ask your server admin to set up email.</Notice>
        ) : null}
        <Link href="/login" className="mt-6 inline-block text-sm text-accent hover:underline">
          Back to sign in
        </Link>
      </AuthShell>
    );
  }
  return (
    <AuthShell title="Reset your password" description="Enter your email and we will send you a link.">
      <form onSubmit={submit} className="space-y-4" noValidate>
        <Field label="Email" htmlFor="email">
          <Input id="email" type="email" autoComplete="email" required value={email} onChange={(e) => setEmail(e.target.value)} />
        </Field>
        {error ? (
          <InlineError>{error}</InlineError>
        ) : null}
        <Button type="submit" className="w-full" disabled={busy || !email.trim()}>
          {busy ? 'Sending…' : 'Send reset link'}
        </Button>
      </form>
      <Link href="/login" className="mt-6 inline-block text-sm text-accent hover:underline">
        Back to sign in
      </Link>
    </AuthShell>
  );
}
