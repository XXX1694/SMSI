'use client';
import { useCallback, useEffect, useState } from 'react';
import { EmptyState, ErrorState, LoadingRows } from '@/components/states';
import { useToast } from '@/components/toast';
import { Button } from '@/components/ui/button';
import { api } from '@/lib/api';
import { actionLabel } from '@/lib/approvals';
import type { Approval } from '@/lib/types';
import { errorMessage, useAsync } from '@/hooks';
import { ApprovalCard } from './approval-card';
import { notifyApprovalsChanged } from './use-pending-approvals';

type Tab = 'pending' | 'all';
const TABS: { id: Tab; label: string }[] = [
  { id: 'pending', label: 'Waiting for you' },
  { id: 'all', label: 'History' },
];
const CLOCK_MS = 30_000;

/** Re-renders every 30 s so "9 min left" counts down and an expired card loses its buttons. */
function useNow(): Date {
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    const t = window.setInterval(() => setNow(new Date()), CLOCK_MS);
    return () => window.clearInterval(t);
  }, []);
  return now;
}

function Empty({ tab }: { tab: Tab }) {
  return tab === 'pending' ? (
    <EmptyState title="Nothing is waiting for you">
      When an agent or API key tries to publish, delete, disconnect, connect an account or schedule within minutes, the request shows up here and
      nothing happens until you decide.
    </EmptyState>
  ) : (
    <EmptyState title="No decisions yet">Approved and denied requests are listed here.</EmptyState>
  );
}

export function ApprovalsView() {
  const toast = useToast();
  const now = useNow();
  const [tab, setTab] = useState<Tab>('pending');
  const [busyId, setBusyId] = useState<string | null>(null);
  const load = useCallback(() => api.approvals.list(tab, 50), [tab]);
  const { data, error, loading, reload } = useAsync(load);

  async function decide(a: Approval, approve: boolean) {
    setBusyId(a.id);
    try {
      await (approve ? api.approvals.approve(a.id) : api.approvals.deny(a.id));
      toast.success(approve ? `Approved: ${actionLabel(a.action)}. The agent can repeat its call now.` : `Denied: ${actionLabel(a.action)}.`);
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      setBusyId(null);
      reload();
      notifyApprovalsChanged();
    }
  }

  let body;
  if (error && !data) body = <ErrorState error={error} onRetry={reload} />;
  else if (loading && !data) body = <LoadingRows rows={3} />;
  else if (!data || data.items.length === 0) body = <Empty tab={tab} />;
  else {
    body = (
      <ul className="space-y-3">
        {data.items.map((a) => (
          <ApprovalCard key={a.id} approval={a} now={now} busy={busyId === a.id} onApprove={(x) => void decide(x, true)} onDeny={(x) => void decide(x, false)} />
        ))}
      </ul>
    );
  }
  return (
    <div className="space-y-4">
      <div role="group" aria-label="Show approvals" className="flex gap-2">
        {TABS.map((t) => (
          <Button key={t.id} size="sm" variant={tab === t.id ? 'primary' : 'secondary'} aria-pressed={tab === t.id} onClick={() => setTab(t.id)}>
            {t.label}
          </Button>
        ))}
      </div>
      {body}
    </div>
  );
}
