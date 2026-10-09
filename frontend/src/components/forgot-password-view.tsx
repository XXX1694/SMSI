'use client';
import Link from 'next/link';
import { useState, type FormEvent } from 'react';
import { AuthShell } from '@/components/auth-shell';
import { InlineError, Notice } from '@/components/states';
import { Button } from '@/components/ui/button';
import { Field, Input } from '@/components/ui/input';
import { useErrorText } from '@/hooks';
import { useTranslations } from '@/i18n/use-translations';
import { api } from '@/lib/api';

export function ForgotPasswordView() {
  const t = useTranslations('auth.forgotPassword');
  const ta = useTranslations('auth');
  const tc = useTranslations('common');
  const errorText = useErrorText();
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
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  }

  if (sent) {
    return (
      <AuthShell title={t('checkTitle')} description={t('checkBody')}>
        {sent.delivery === 'log' ? (
          <Notice tone="warning">{t('noMail')}</Notice>
        ) : null}
        <Link href="/login" className="mt-6 inline-block text-sm text-accent hover:underline">
          {ta('backToSignIn')}
        </Link>
      </AuthShell>
    );
  }
  return (
    <AuthShell title={t('title')} description={t('intro')}>
      <form onSubmit={submit} className="space-y-4" noValidate>
        <Field label={ta('email')} htmlFor="email">
          <Input id="email" type="email" autoComplete="email" required value={email} onChange={(e) => setEmail(e.target.value)} />
        </Field>
        {error ? (
          <InlineError>{error}</InlineError>
        ) : null}
        <Button type="submit" className="w-full" disabled={busy || !email.trim()}>
          {busy ? tc('sending') : t('send')}
        </Button>
      </form>
      <Link href="/login" className="mt-6 inline-block text-sm text-accent hover:underline">
        {ta('backToSignIn')}
      </Link>
    </AuthShell>
  );
}
