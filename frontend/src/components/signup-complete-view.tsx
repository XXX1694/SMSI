'use client';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useCallback, useEffect, useState, type FormEvent } from 'react';
import { useAuth } from '@/components/auth-provider';
import { AuthShell } from '@/components/auth-shell';
import { ErrorState, InlineError, LoadingRows } from '@/components/states';
import { TermsCheckbox } from '@/components/terms-checkbox';
import { Button } from '@/components/ui/button';
import { Field, Input } from '@/components/ui/input';
import { useAsync, useErrorText } from '@/hooks';
import { useTranslations } from '@/i18n/use-translations';
import { ApiError, api } from '@/lib/api';
import { safeNext } from '@/lib/safe-next';
import { signInProviderName } from '@/lib/sign-in-providers';
import type { PendingSignup } from '@/lib/types';

const EXPIRED = '/login?error=signup_expired';

/** The form: email read-only (the provider verified it), the name editable, and the Terms accepted explicitly (D-016). */
function SignupForm({ pending }: { pending: PendingSignup }) {
  const t = useTranslations('auth.signupComplete');
  const ta = useTranslations('auth');
  const errorText = useErrorText();
  const router = useRouter();
  const { completeSignup } = useAuth();
  const [name, setName] = useState(pending.display_name);
  const [accepted, setAccepted] = useState(false);
  const [termsError, setTermsError] = useState(false);
  const [nameError, setNameError] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const provider = signInProviderName(pending.provider) ?? ta('providerFallback');

  async function submit(e: FormEvent) {
    e.preventDefault();
    setFormError(null);
    setNameError(false);
    if (!accepted) {
      setTermsError(true);
      document.getElementById('accept-terms')?.focus();
      return;
    }
    setBusy(true);
    try {
      await completeSignup(name.trim(), true);
      router.replace(safeNext(pending.next) ?? '/dashboard');
    } catch (err) {
      if (err instanceof ApiError && err.status === 404) router.replace(EXPIRED);
      else if (err instanceof ApiError && err.fields.display_name) setNameError(true);
      else setFormError(errorText(err));
      setBusy(false);
    }
  }

  return (
    <AuthShell title={t('title')} description={t('intro', { provider })}>
      <form onSubmit={submit} className="space-y-4" noValidate>
        <Field label={ta('email')} htmlFor="email" hint={t('emailHint', { provider })}>
          <Input id="email" type="email" value={pending.email} readOnly autoComplete="email" className="max-md:h-11" />
        </Field>
        <Field label={ta('name')} htmlFor="name" optional error={nameError ? t('nameTooLong') : undefined}>
          <Input id="name" autoComplete="name" className="max-md:h-11" value={name} onChange={(e) => setName(e.target.value)} />
        </Field>
        <TermsCheckbox
          checked={accepted}
          onCheckedChange={(v) => {
            setAccepted(v);
            if (v) setTermsError(false);
          }}
          error={termsError ? ta('acceptTerms') : undefined}
        />
        {formError ? <InlineError>{formError}</InlineError> : null}
        <Button type="submit" className="w-full" loading={busy}>
          {busy ? ta('creating') : ta('createAccount')}
        </Button>
      </form>
      <Link href="/login" className="mt-6 inline-flex min-h-11 items-center text-sm text-accent hover:underline md:min-h-0">
        {ta('backToSignIn')}
      </Link>
    </AuthShell>
  );
}

/**
 * The last step of "Continue with Google / GitHub" for a new person: no account exists yet, so they see what will be
 * created before it is. With no waiting sign-up (expired, used, opened directly) the way back is the sign-in page.
 */
export function SignupCompleteView() {
  const t = useTranslations('auth.signupComplete');
  const router = useRouter();
  const load = useCallback(() => api.auth.pendingSignup(), []);
  const { data: pending, error, loading, reload } = useAsync(load);
  const gone = error instanceof ApiError && error.status === 404;

  useEffect(() => {
    if (gone) router.replace(EXPIRED);
  }, [gone, router]);

  if (error && !gone) {
    return (
      <AuthShell title={t('title')}>
        <ErrorState error={error} onRetry={reload} />
      </AuthShell>
    );
  }
  if (loading || gone || !pending) {
    return (
      <AuthShell title={t('title')}>
        <LoadingRows rows={3} />
      </AuthShell>
    );
  }
  return <SignupForm pending={pending} />;
}
