'use client';
import { useState, type FormEvent } from 'react';
import { useAuth } from '@/components/auth-provider';
import { InlineError, Notice } from '@/components/states';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogFooter } from '@/components/ui/dialog';
import { ProviderButtons, useSignInProviders } from '@/components/social-sign-in';
import { Field, Input } from '@/components/ui/input';
import { PasswordInput } from '@/components/ui/password-input';
import { ApiError, api } from '@/lib/api';
import { useErrorText } from '@/hooks';
import { useTranslations } from '@/i18n/use-translations';

/** Which form field the API refused (`fields`), or the generic message. */
function refusal(e: unknown, errorText: (e: unknown) => string): { password?: boolean; confirm?: boolean; reauth?: boolean; general?: string } {
  // A user who signed up with Google or GitHub has no password: an old session must be refreshed by signing in again.
  if (e instanceof ApiError && e.code === 'REAUTH_REQUIRED') return { reauth: true };
  if (e instanceof ApiError && e.status === 400) {
    if (e.fields.password) return { password: true };
    if (e.fields.confirm) return { confirm: true };
  }
  return { general: errorText(e) };
}

/**
 * A session too old to delete a password-less account: sign in again with the provider and come back to Settings, where
 * the dialog can be opened again with a fresh session. The provider list loads only now, not on every visit to Settings.
 */
function ReauthNotice() {
  const t = useTranslations();
  const errorText = useErrorText();
  const { providers, error, loading, reload } = useSignInProviders();
  return (
    <Notice tone="warning">
      <p>{t('settings.deleteAccount.reauthBody')}</p>
      <div className="mt-3">
        {loading ? <p role="status">{t('common.loading')}</p> : null}
        {error ? <InlineError onRetry={reload}>{errorText(error)}</InlineError> : null}
        <ProviderButtons providers={providers} next="/settings" intent="again" />
      </div>
    </Notice>
  );
}

/** "Delete account": password plus the typed email, then a grace period during which signing in cancels it (D-019). */
export function DeleteAccount() {
  const t = useTranslations();
  const errorText = useErrorText();
  const { user, endSession } = useAuth();
  const graceDays = user?.deletion_grace_days ?? 7;
  const [open, setOpen] = useState(false);
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState<ReturnType<typeof refusal>>({});
  const matches = confirm.trim().toLowerCase() === (user?.email ?? '').toLowerCase();

  function reset(next: boolean) {
    setOpen(next);
    setPassword('');
    setConfirm('');
    setProblem({});
  }

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setProblem({});
    try {
      await api.account.requestDeletion(password, confirm.trim());
      // The (app) layout sees the missing user and sends the owner to the login page with the notice.
      endSession('deleted');
    } catch (err) {
      setProblem(refusal(err, errorText));
      setBusy(false);
    }
  }

  return (
    <div className="space-y-3">
      <p className="text-sm text-muted-foreground">{t('settings.deleteAccount.intro', { days: graceDays })}</p>
      <Button variant="danger" onClick={() => reset(true)}>
        {t('settings.deleteAccount.open')}
      </Button>
      <Dialog open={open} onOpenChange={(o) => !busy && reset(o)}>
        <DialogContent title={t('settings.deleteAccount.dialogTitle')} description={t('settings.deleteAccount.dialogBody')}>
          <form onSubmit={submit} className="space-y-4" noValidate aria-label={t('settings.deleteAccount.formLabel')}>
            <Field
              label={t('settings.deleteAccount.password')}
              htmlFor="del-password"
              hint={t('settings.deleteAccount.passwordHint')}
              error={problem.password ? t('settings.deleteAccount.wrongPassword') : undefined}
            >
              <PasswordInput id="del-password" label={t('settings.deleteAccount.password')} autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} />
            </Field>
            <Field
              label={t('settings.deleteAccount.confirmLabel', { email: user?.email ?? t('settings.deleteAccount.yourEmail') })}
              htmlFor="del-confirm"
              error={problem.confirm ? t('settings.deleteAccount.wrongConfirm') : undefined}
            >
              <Input id="del-confirm" autoComplete="off" autoCapitalize="off" spellCheck={false} value={confirm} onChange={(e) => setConfirm(e.target.value)} />
            </Field>
            {problem.reauth ? <ReauthNotice /> : null}
            {problem.general ? <InlineError>{problem.general}</InlineError> : null}
            <DialogFooter>
              <Button type="button" variant="secondary" onClick={() => reset(false)} disabled={busy}>
                {t('common.cancel')}
              </Button>
              <Button type="submit" variant="danger" disabled={busy || !matches}>
                {busy ? t('common.working') : t('settings.deleteAccount.submit')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}
