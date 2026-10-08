'use client';
import { useState, type FormEvent } from 'react';
import { RevokeKeysOption } from '@/components/revoke-keys-option';
import { useToast } from '@/components/toast';
import { Button } from '@/components/ui/button';
import { Field, Input } from '@/components/ui/input';
import { errorMessage } from '@/hooks';
import { api } from '@/lib/api';

/** Change the password of the signed-in user. Every other session is signed out by the server. */
export function PasswordForm() {
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
      setError('The new password must be 8 to 128 characters.');
      return;
    }
    if (next !== confirm) {
      setError('The two new passwords do not match.');
      return;
    }
    setBusy(true);
    try {
      await api.auth.changePassword(current, next, revokeKeys);
      setCurrent('');
      setNext('');
      setConfirm('');
      toast.success(
        revokeKeys
          ? 'Password changed. Your other sessions, API keys and MCP connections were revoked.'
          : 'Password changed. Your other sessions were signed out; API keys and MCP connections were not revoked.',
      );
      setRevokeKeys(false);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit} className="space-y-4" noValidate aria-label="Change password">
      <Field label="Current password" htmlFor="pw-current">
        <Input id="pw-current" type="password" autoComplete="current-password" value={current} onChange={(e) => setCurrent(e.target.value)} />
      </Field>
      <Field label="New password" htmlFor="pw-new" hint="At least 8 characters. Your other browser sessions are signed out when you save.">
        <Input id="pw-new" type="password" autoComplete="new-password" value={next} onChange={(e) => setNext(e.target.value)} />
      </Field>
      <Field label="Repeat the new password" htmlFor="pw-confirm">
        <Input id="pw-confirm" type="password" autoComplete="new-password" value={confirm} onChange={(e) => setConfirm(e.target.value)} />
      </Field>
      <RevokeKeysOption id="pw-revoke-keys" checked={revokeKeys} onChange={setRevokeKeys} />
      {error ? (
        <p role="alert" className="text-sm text-danger">
          {error}
        </p>
      ) : null}
      <Button type="submit" disabled={busy || !current || !next || !confirm}>
        {busy ? 'Saving…' : 'Change password'}
      </Button>
    </form>
  );
}
