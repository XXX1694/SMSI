'use client';
import { useRouter } from 'next/navigation';
import { useState, type FormEvent } from 'react';
import { useAuth } from '@/components/auth-provider';
import { InlineError } from '@/components/states';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogFooter } from '@/components/ui/dialog';
import { Field, Input } from '@/components/ui/input';
import { ApiError, api } from '@/lib/api';
import { errorMessage } from '@/hooks';

/** Why a form field was refused, from the API's `fields`, or the generic message. */
function refusal(e: unknown): { password?: string; confirm?: string; general?: string } {
  if (e instanceof ApiError && e.status === 400) {
    if (e.fields.password) return { password: 'That is not your current password.' };
    if (e.fields.confirm) return { confirm: 'Type your email address exactly as shown.' };
  }
  return { general: errorMessage(e) };
}

/** "Delete account": password plus the typed email, then a grace period during which signing in cancels it (D-019). */
export function DeleteAccount() {
  const { user, endSession } = useAuth();
  const graceDays = user?.deletion_grace_days ?? 7;
  const router = useRouter();
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
      endSession();
      router.replace('/login?deleted=1');
    } catch (err) {
      setProblem(refusal(err));
      setBusy(false);
    }
  }

  return (
    <div className="space-y-3">
      <p className="text-sm text-muted-foreground">
        Deletes your account, posts, media, connected accounts, API keys and audit log. It happens {graceDays} days after you confirm. Until then you can sign in and cancel;
        scheduled posts are set back to drafts and every session and API key is signed out right away. Download your data first if you want a copy.
      </p>
      <Button variant="danger" onClick={() => reset(true)}>
        Delete account…
      </Button>
      <Dialog open={open} onOpenChange={(o) => !busy && reset(o)}>
        <DialogContent title="Delete your account?" description="Everything you own in SocialOS is deleted after the grace period. This cannot be undone.">
          <form onSubmit={submit} className="space-y-4" noValidate aria-label="Delete account">
            <Field label="Your password" htmlFor="del-password" error={problem.password}>
              <Input id="del-password" type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} />
            </Field>
            <Field label={`Type ${user?.email ?? 'your email'} to confirm`} htmlFor="del-confirm" error={problem.confirm}>
              <Input id="del-confirm" autoComplete="off" autoCapitalize="off" spellCheck={false} value={confirm} onChange={(e) => setConfirm(e.target.value)} />
            </Field>
            {problem.general ? <InlineError>{problem.general}</InlineError> : null}
            <DialogFooter>
              <Button type="button" variant="secondary" onClick={() => reset(false)} disabled={busy}>
                Cancel
              </Button>
              <Button type="submit" variant="danger" disabled={busy || !password || !matches}>
                {busy ? 'Working…' : 'Delete my account'}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}
