'use client';
import { useState, type FormEvent } from 'react';
import { InlineError } from '@/components/states';
import { useToast } from '@/components/toast';
import { ReauthNotice, linkedProviders } from '@/components/reauth-notice';
import { useAuth } from '@/components/auth-provider';
import { Button } from '@/components/ui/button';
import { Field } from '@/components/ui/input';
import { PasswordInput } from '@/components/ui/password-input';
import { useErrorText } from '@/hooks';
import { useTranslations } from '@/i18n/use-translations';
import { ApiError, api } from '@/lib/api';

/** The first password of an account made with Google or GitHub. Changing an existing one is `PasswordForm`. */
export function SetPasswordForm({ onDone, onCancel }: { onDone: () => void; onCancel: () => void }) {
  const t = useTranslations('settings.signIn');
  const ta = useTranslations('auth');
  const tc = useTranslations('common');
  const errorText = useErrorText();
  const toast = useToast();
  const { user } = useAuth();
  const [password, setPassword] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [reauth, setReauth] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    setReauth(false);
    if (password.length < 8 || password.length > 128) {
      setError(ta('passwordLength'));
      return;
    }
    setBusy(true);
    try {
      await api.auth.setPassword(password);
      toast.success(t('passwordSetDone'));
      onDone();
    } catch (err) {
      if (err instanceof ApiError && err.code === 'REAUTH_REQUIRED') setReauth(true);
      else setError(errorText(err));
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit} className="space-y-3" noValidate aria-label={t('setPasswordOpen')}>
      <Field label={ta('newPassword')} htmlFor="set-password" hint={t('newPasswordHint')}>
        <PasswordInput id="set-password" label={ta('newPassword')} autoComplete="new-password" value={password} onChange={(e) => setPassword(e.target.value)} />
      </Field>
      {reauth ? <ReauthNotice body={t('reauthBody')} linked={linkedProviders(user?.login_methods)} /> : null}
      {error ? <InlineError>{error}</InlineError> : null}
      <div className="flex flex-wrap gap-2">
        <Button type="submit" disabled={busy || !password}>
          {busy ? tc('working') : t('setPasswordSubmit')}
        </Button>
        <Button type="button" variant="secondary" onClick={onCancel} disabled={busy}>
          {tc('cancel')}
        </Button>
      </div>
    </form>
  );
}
