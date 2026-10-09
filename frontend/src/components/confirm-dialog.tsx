'use client';
import { useState, type ReactNode } from 'react';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogFooter } from '@/components/ui/dialog';
import { InlineError } from '@/components/states';
import { errorMessage } from '@/hooks';

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description: string;
  confirmLabel: string;
  /** Label of the button that closes the dialog; "Cancel" by default. */
  dismissLabel?: string;
  destructive?: boolean;
  onConfirm: () => Promise<void>;
  children?: ReactNode;
}

export function ConfirmDialog({ open, onOpenChange, title, description, confirmLabel, dismissLabel, destructive, onConfirm, children }: Props) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function run() {
    setBusy(true);
    setError(null);
    try {
      await onConfirm();
      onOpenChange(false);
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        if (!busy) {
          setError(null);
          onOpenChange(o);
        }
      }}
    >
      <DialogContent title={title} description={description}>
        {children}
        {error ? (
          <InlineError className="mt-3">{error}</InlineError>
        ) : null}
        <DialogFooter>
          <Button variant="secondary" onClick={() => onOpenChange(false)} disabled={busy}>
            {dismissLabel ?? 'Cancel'}
          </Button>
          <Button variant={destructive ? 'danger' : 'primary'} onClick={run} disabled={busy}>
            {busy ? 'Working…' : confirmLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
