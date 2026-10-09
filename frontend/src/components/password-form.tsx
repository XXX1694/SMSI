'use client';
import { useState, type FormEvent } from 'react';
import { RevokeKeysOption } from '@/components/revoke-keys-option';
import { useToast } from '@/components/toast';
import { Button } from '@/components/ui/button';
import { Field, Input } from '@/components/ui/input';
import { InlineError } from '@/components/states';
import { useErrorText } from '@/hooks';
import { useTranslations } from '@/i18n/use-translations';
import { api } from '@/lib/api';

/** Change the password of the signed-in user. Every other session is signed out by the server. */
export function PasswordForm() {
  const t = useTranslations('settings');
  const ta = useTranslations('auth');
  const tc = useTranslations('common');
  const errorText = useErrorText();
  const toast = useToast();
  const [current, setCurrent] = useState('');
  const [next, setNext] = useState('');
  const [confirm, setConfirm] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [revokeKeys, setRevokeKeys] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    if (next.length < 8 || next.length > 128) {
      setError(t('passwordLength'));
      return;
    }
    if (next !== confirm) {
      setError(t('passwordMismatch'));
      return;
    }
    setBusy(true);
    try {
      await api.auth.changePassword(current, next, revokeKeys);
      setCurrent('');
      setNext('');
      setConfirm('');
      toast.success(
        revokeKeys ? t('passwordChangedRevoked') : t('passwordChangedKept'),
      );
      setRevokeKeys(false);
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit} className="space-y-4" noValidate aria-label={t('changePassword')}>
      <Field label={t('currentPassword')} htmlFor="pw-current">
        <Input id="pw-current" type="password" autoComplete="current-password" value={current} onChange={(e) => setCurrent(e.target.value)} />
      </Field>
      <Field label={ta('newPassword')} htmlFor="pw-new" hint={t('newPasswordHint')}>
        <Input id="pw-new" type="password" autoComplete="new-password" value={next} onChange={(e) => setNext(e.target.value)} />
      </Field>
      <Field label={ta('repeatPassword')} htmlFor="pw-confirm">
        <Input id="pw-confirm" type="password" autoComplete="new-password" value={confirm} onChange={(e) => setConfirm(e.target.value)} />
      </Field>
      <RevokeKeysOption id="pw-revoke-keys" checked={revokeKeys} onChange={setRevokeKeys} />
      {error ? (
        <InlineError>{error}</InlineError>
      ) : null}
      <Button type="submit" disabled={busy || !current || !next || !confirm}>
        {busy ? tc('saving') : t('changePassword')}
      </Button>
    </form>
  );
}
