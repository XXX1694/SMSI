'use client';
import { useRouter, useSearchParams } from 'next/navigation';
import { useCallback, useEffect, useState } from 'react';
import { EmptyState, ErrorState, LoadingRows } from '@/components/states';
import { useToast } from '@/components/toast';
import { Button } from '@/components/ui/button';
import { api } from '@/lib/api';
import { actionLabel } from '@/lib/approvals';
import type { Approval } from '@/lib/types';
import { useAsync, useErrorText } from '@/hooks';
import { ApprovalCard } from './approval-card';
import { notifyApprovalsChanged } from './use-pending-approvals';
import { useTranslations } from '@/i18n/use-translations';

type Tab = 'pending' | 'all';
const TABS = [
  { id: 'pending', labelKey: 'tabWaiting' },
  { id: 'all', labelKey: 'tabHistory' },
] as const;
const CLOCK_MS = 30_000;

/** Re-renders every 30 s so "9 min left" counts down and an expired card loses its buttons. */
function useNow(): Date {
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    const timer = window.setInterval(() => setNow(new Date()), CLOCK_MS);
    return () => window.clearInterval(timer);
  }, []);
  return now;
}

function Empty({ tab }: { tab: Tab }) {
  const t = useTranslations('approvals');
  return tab === 'pending' ? (
    <EmptyState title={t('emptyPendingTitle')}>{t('emptyPendingBody')}</EmptyState>
  ) : (
    <EmptyState title={t('emptyHistoryTitle')}>{t('emptyHistoryBody')}</EmptyState>
  );
}

export function ApprovalsView() {
  const t = useTranslations();
  const errorText = useErrorText();
  const toast = useToast();
  const now = useNow();
  const router = useRouter();
  const params = useSearchParams();
  // The tab lives in the URL (?tab=history) so a reload or a shared link opens the same list.
  const tab: Tab = params.get('tab') === 'history' ? 'all' : 'pending';
  const setTab = (next: Tab) => router.replace(next === 'all' ? '/approvals?tab=history' : '/approvals');
  const [busyId, setBusyId] = useState<string | null>(null);
  const load = useCallback(() => api.approvals.list(tab, 50), [tab]);
  const { data, error, loading, reload } = useAsync(load);

  async function decide(a: Approval, approve: boolean) {
    setBusyId(a.id);
    try {
      await (approve ? api.approvals.approve(a.id) : api.approvals.deny(a.id));
      const action = actionLabel(a.action, t);
      toast.success(approve ? t('approvals.approvedToast', { action, agent: a.actor_label }) : t('approvals.deniedToast', { action }));
    } catch (e) {
      toast.error(errorText(e));
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
      <div role="group" aria-label={t('approvals.showLabel')} className="flex gap-2">
        {TABS.map((x) => (
          <Button key={x.id} size="sm" variant={tab === x.id ? 'primary' : 'secondary'} aria-pressed={tab === x.id} onClick={() => setTab(x.id)}>
            {t(`approvals.${x.labelKey}`)}
          </Button>
        ))}
      </div>
      {body}
    </div>
  );
}
