'use client';
import { Check, X } from 'lucide-react';
import { useId, useState } from 'react';
import { usePrefs } from '@/components/prefs-provider';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { actionLabel, isIrreversible, isOpen, summaryLines, timeLeft, type SummaryLine } from '@/lib/approvals';
import { formatDateTime } from '@/lib/time';
import { cn } from '@/lib/utils';
import type { Approval, ApprovalStatus } from '@/lib/types';

const STATUS: Record<ApprovalStatus, { label: string; tone: 'success' | 'danger' | 'neutral' | 'warning' }> = {
  pending: { label: 'Waiting', tone: 'warning' },
  approved: { label: 'Approved, not used yet', tone: 'success' },
  consumed: { label: 'Approved and used', tone: 'success' },
  denied: { label: 'Denied', tone: 'danger' },
  expired: { label: 'Expired', tone: 'neutral' },
};

interface Props {
  approval: Approval;
  now: Date;
  /** The id being decided right now (all buttons of that card are disabled). */
  busy: boolean;
  onApprove: (a: Approval) => void;
  onDeny: (a: Approval) => void;
}

/** One line of the summary. Long text is clamped; "Show full text" reveals all of it, as plain text. */
function SummaryRow({ line }: { line: SummaryLine }) {
  const [open, setOpen] = useState(false);
  const id = useId();
  return (
    <div className="flex flex-col gap-0.5 sm:flex-row sm:gap-3">
      <dt className="shrink-0 text-muted-foreground sm:w-40">{line.label}</dt>
      <dd className="min-w-0 flex-1 break-words">
        <span id={id} className={cn('whitespace-pre-line', line.long && !open && 'line-clamp-4')}>
          {line.value}
        </span>
        {line.long ? (
          <button
            type="button"
            className="mt-1 flex min-h-6 items-center text-xs font-medium max-md:min-h-11 text-accent underline-offset-4 hover:underline"
            aria-expanded={open}
            aria-controls={id}
            onClick={() => setOpen((o) => !o)}
          >
            {open ? 'Show less' : 'Show full text'}
          </button>
        ) : null}
      </dd>
    </div>
  );
}

function Summary({ approval }: { approval: Approval }) {
  const { timezone } = usePrefs();
  const lines = summaryLines(approval, timezone);
  if (lines.length === 0) return null;
  return (
    <dl className="mt-3 space-y-2 text-sm">
      {lines.map((l, i) => (
        <SummaryRow key={`${i}-${l.label}`} line={l} />
      ))}
    </dl>
  );
}

export function ApprovalCard({ approval, now, busy, onApprove, onDeny }: Props) {
  const { timezone } = usePrefs();
  const open = isOpen(approval, now);
  const label = actionLabel(approval.action);
  const state = STATUS[approval.status === 'pending' && !open ? 'expired' : approval.status];
  return (
    <Card as="li" className="card-lift">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex flex-wrap items-center gap-2">
          <Badge tone={isIrreversible(approval.action) ? 'danger' : 'accent'}>{label}</Badge>
          <span className="text-sm text-muted-foreground">
            asked by <span className="font-medium text-foreground">{approval.actor_label}</span>
          </span>
        </div>
        <span className="text-xs text-muted-foreground">
          {open ? timeLeft(approval.expires_at, now) : approval.decided_at ? formatDateTime(approval.decided_at, timezone) : formatDateTime(approval.created_at, timezone)}
        </span>
      </div>
      <Summary approval={approval} />
      {open ? (
        <div className="mt-4 flex flex-wrap items-center justify-end gap-2">
          <Button variant="secondary" size="sm" disabled={busy} onClick={() => onDeny(approval)} aria-label={`Deny: ${label}`}>
            <X className="h-4 w-4" aria-hidden /> Deny
          </Button>
          <Button variant={isIrreversible(approval.action) ? 'danger' : 'primary'} size="sm" disabled={busy} onClick={() => onApprove(approval)} aria-label={`Approve: ${label}`}>
            <Check className="h-4 w-4" aria-hidden /> {busy ? 'Working…' : 'Approve'}
          </Button>
        </div>
      ) : (
        <div className="mt-3">
          <Badge tone={state.tone}>{state.label}</Badge>
        </div>
      )}
    </Card>
  );
}
