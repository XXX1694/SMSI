'use client';
import { useState, type FormEvent } from 'react';
import { linkedProviders, ReauthNotice } from '@/components/reauth-notice';
import { InlineError } from '@/components/states';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogFooter } from '@/components/ui/dialog';
import { Field } from '@/components/ui/input';
import { PasswordInput } from '@/components/ui/password-input';
import { useAuth } from '@/components/auth-provider';
import { useErrorText } from '@/hooks';
import { useTranslations } from '@/i18n/use-translations';
import { ApiError } from '@/lib/api';

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description: string;
  confirmLabel: string;
  destructive?: boolean;
  /** Users with a password must type it (the server checks it); users without one are asked for nothing. */
  needsPassword: boolean;
  /** What a refusal with this HTTP status means for this action; other failures use the catalog's wording for the error code. */
  statusText?: Partial<Record<number, string>>;
  onConfirm: (password: string | undefined) => Promise<void>;
}

/** Thrown by `onConfirm` for a failure that has its own, already translated sentence. */
export class ReauthFailure extends Error {}

type Problem = { password?: boolean; reauth?: boolean; general?: string };

/**
 * A confirm dialog for changes to how the account can be entered (connect or disconnect a provider). The server
 * re-authenticates: a password for users who have one, a fresh session for users who do not (REAUTH_REQUIRED, then the
 * sign-in-again buttons appear in the dialog).
 */
export function ReauthDialog({ open, onOpenChange, title, description, confirmLabel, destructive, needsPassword, statusText, onConfirm }: Props) {
  const t = useTranslations('settings.signIn');
  const tc = useTranslations('common');
  const errorText = useErrorText();
  const { user } = useAuth();
  const [password, setPassword] = useState('');
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState<Problem>({});

  function close(next: boolean) {
    if (busy) return;
    setPassword('');
    setProblem({});
    onOpenChange(next);
  }

  function refusal(e: unknown): Problem {
    if (e instanceof ReauthFailure) return { general: e.message };
    if (e instanceof ApiError) {
      if (e.code === 'REAUTH_REQUIRED') return { reauth: true };
      if (e.status === 400 && e.fields.current_password) return { password: true };
      const own = statusText?.[e.status];
      if (own) return { general: own };
    }
    return { general: errorText(e) };
  }

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setProblem({});
    try {
      await onConfirm(needsPassword ? password : undefined);
      setPassword('');
      onOpenChange(false);
    } catch (err) {
      setProblem(refusal(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={close}>
      <DialogContent title={title} description={description}>
        <form onSubmit={submit} className="space-y-4" noValidate aria-label={title}>
          {needsPassword ? (
            <Field label={t('dialogPassword')} htmlFor="reauth-password" error={problem.password ? t('wrongPassword') : undefined}>
              <PasswordInput id="reauth-password" label={t('dialogPassword')} autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} />
            </Field>
          ) : null}
          {problem.reauth ? <ReauthNotice body={t('reauthBody')} linked={linkedProviders(user?.login_methods)} /> : null}
          {problem.general ? <InlineError>{problem.general}</InlineError> : null}
          <DialogFooter>
            <Button type="button" variant="secondary" onClick={() => close(false)} disabled={busy}>
              {tc('cancel')}
            </Button>
            <Button type="submit" variant={destructive ? 'danger' : 'primary'} disabled={busy || (needsPassword && !password)}>
              {busy ? tc('working') : confirmLabel}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
