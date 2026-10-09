'use client';
import Link from 'next/link';
import { InlineError } from '@/components/states';
import { TermsCheckbox } from '@/components/terms-checkbox';
import { Button } from '@/components/ui/button';
import { Field, Input } from '@/components/ui/input';
import { PasswordInput } from '@/components/ui/password-input';
import { useAuthEmailForm } from '@/components/use-auth-email-form';
import { useTranslations } from '@/i18n/use-translations';
import { DEMO } from '@/lib/demo/config';
import { withNext } from '@/lib/safe-next';

const LINK = 'inline-flex min-h-11 items-center text-accent hover:underline md:min-h-0';

/** The registration was refused because the email is taken: the way forward is signing in or resetting the password. */
function DuplicateEmail({ next }: { next: string | null }) {
  const t = useTranslations('auth');
  return (
    <div role="alert" className="space-y-2 rounded-md border border-danger/30 bg-danger-soft px-3 py-2 text-sm">
      <p className="text-danger">{t('duplicateEmail')}</p>
      <p className="flex flex-wrap gap-x-4">
        <Link href={withNext('/login', next)} className={`${LINK} font-medium`}>
          {t('signIn')}
        </Link>
        {!DEMO ? (
          <Link href="/forgot-password" className={LINK}>
            {t('forgot')}
          </Link>
        ) : null}
      </p>
    </div>
  );
}

/** Email and password for `/login` and `/register`, with per-field errors, a show/hide toggle and the Terms checkbox. */
export function AuthEmailForm({ mode, next }: { mode: 'login' | 'register'; next: string | null }) {
  const t = useTranslations('auth');
  const f = useAuthEmailForm(mode, next);
  return (
    <form ref={f.form} onSubmit={f.submit} className="space-y-4" noValidate>
      <Field label={t('email')} htmlFor="email" error={f.errors.email} announce={false} required>
        <Input
          id="email"
          type="email"
          inputMode="email"
          autoComplete={f.isLogin ? 'username' : 'email'}
          autoCapitalize="none"
          autoCorrect="off"
          spellCheck={false}
          className="max-md:h-11"
          value={f.email}
          onChange={(e) => f.setEmail(e.target.value)}
          onBlur={f.blur('email')}
        />
      </Field>
      {f.duplicate ? <DuplicateEmail next={next} /> : null}
      <Field label={t('password')} htmlFor="password" error={f.errors.password} announce={false} hint={f.isLogin ? undefined : t('passwordHint')} required>
        <PasswordInput
          id="password"
          label={t('password')}
          autoComplete={f.isLogin ? 'current-password' : 'new-password'}
          className="max-md:h-11"
          value={f.password}
          onChange={(e) => f.setPassword(e.target.value)}
          onBlur={f.blur('password')}
        />
      </Field>
      {f.isLogin && !DEMO ? (
        <p className="-mt-2 text-right text-xs">
          <Link href="/forgot-password" className={LINK}>
            {t('forgot')}
          </Link>
        </p>
      ) : null}
      {!f.isLogin ? <TermsCheckbox checked={f.accepted} onCheckedChange={f.accept} error={f.errors.terms} /> : null}
      {f.formError ? <InlineError>{f.formError}</InlineError> : null}
      <Button type="submit" className="w-full" loading={f.busy}>
        {f.busy ? (f.isLogin ? t('signingIn') : t('creating')) : f.isLogin ? t('signIn') : t('createAccount')}
      </Button>
    </form>
  );
}
