'use client';
import { useState, type FormEvent } from 'react';
import { useAuth } from '@/components/auth-provider';
import { InlineError } from '@/components/states';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogFooter } from '@/components/ui/dialog';
import { Field, Input } from '@/components/ui/input';
import { ApiError, api } from '@/lib/api';
import { useErrorText } from '@/hooks';
import { useTranslations } from '@/i18n/use-translations';

/** Which form field the API refused (`fields`), or the generic message. */
function refusal(e: unknown, errorText: (e: unknown) => string): { password?: boolean; confirm?: boolean; general?: string } {
  if (e instanceof ApiError && e.status === 400) {
    if (e.fields.password) return { password: true };
    if (e.fields.confirm) return { confirm: true };
  }
  return { general: errorText(e) };
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
            <Field label={t('settings.deleteAccount.password')} htmlFor="del-password" error={problem.password ? t('settings.deleteAccount.wrongPassword') : undefined}>
              <Input id="del-password" type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} />
            </Field>
            <Field
              label={t('settings.deleteAccount.confirmLabel', { email: user?.email ?? t('settings.deleteAccount.yourEmail') })}
              htmlFor="del-confirm"
              error={problem.confirm ? t('settings.deleteAccount.wrongConfirm') : undefined}
            >
              <Input id="del-confirm" autoComplete="off" autoCapitalize="off" spellCheck={false} value={confirm} onChange={(e) => setConfirm(e.target.value)} />
            </Field>
            {problem.general ? <InlineError>{problem.general}</InlineError> : null}
            <DialogFooter>
              <Button type="button" variant="secondary" onClick={() => reset(false)} disabled={busy}>
                {t('common.cancel')}
              </Button>
              <Button type="submit" variant="danger" disabled={busy || !password || !matches}>
                {busy ? t('common.working') : t('settings.deleteAccount.submit')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}
