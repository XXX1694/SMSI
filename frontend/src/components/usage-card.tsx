'use client';
import { useCallback } from 'react';
import { ErrorState, LoadingRows } from '@/components/states';
import { api } from '@/lib/api';
import { formatBytes } from '@/lib/media';
import { formatDateTime } from '@/lib/time';
import type { QuotaLine, UsageReport } from '@/lib/types';
import { useAsync } from '@/hooks';
import { usePrefs } from '@/components/prefs-provider';

type Row = { key: string; label: string; line: QuotaLine; fmt: (n: number) => string };

function rows(u: UsageReport): Row[] {
  const n = (v: number) => v.toLocaleString();
  return [
    { key: 'accounts', label: 'Connected accounts', line: u.quotas.connected_accounts, fmt: n },
    { key: 'posts', label: 'Posts this month', line: u.quotas.scheduled_posts_month, fmt: n },
    { key: 'media', label: 'Media storage', line: u.quotas.media_bytes, fmt: formatBytes },
  ];
}

function Meter({ row }: { row: Row }) {
  const { line, fmt, label } = row;
  const used = line.used ?? 0;
  const unlimited = line.limit < 0;
  const ratio = unlimited || line.limit === 0 ? 0 : Math.min(1, used / line.limit);
  const full = !unlimited && used >= line.limit;
  return (
    <div>
      <div className="flex items-baseline justify-between gap-3 text-sm">
        <span>{label}</span>
        <span className={full ? 'font-medium text-warning' : 'text-muted-foreground'}>
          {unlimited ? `${fmt(used)} · no limit` : `${fmt(used)} of ${fmt(line.limit)}`}
          {full ? ' · limit reached' : ''}
        </span>
      </div>
      {unlimited ? null : (
        <div
          role="progressbar"
          aria-label={label}
          aria-valuemin={0}
          aria-valuemax={line.limit}
          aria-valuenow={Math.min(used, line.limit)}
          className="mt-1.5 h-1.5 overflow-hidden rounded-full bg-muted"
        >
          <div className={`h-full rounded-full ${full || ratio >= 0.8 ? 'bg-warning' : 'bg-foreground/70'}`} style={{ width: `${Math.round(ratio * 100)}%` }} />
        </div>
      )}
    </div>
  );
}

/** "Plan & usage": what the account has used against the limits of its plan (D-014). */
export function UsageCard() {
  const { timezone } = usePrefs();
  const load = useCallback(() => api.account.usage(), []);
  const { data, error, loading, reload } = useAsync(load);
  if (loading && !data) return <LoadingRows rows={3} />;
  if (error || !data) return <ErrorState error={error} onRetry={reload} title="Could not load your usage" />;
  const rpm = data.quotas.agent_requests_per_minute.limit;
  return (
    <div className="space-y-4">
      <p className="text-sm text-muted-foreground">
        Plan <span className="font-medium capitalize text-foreground">{data.plan}</span>. Posts are counted from {formatDateTime(data.period_start, timezone)} to{' '}
        {formatDateTime(data.period_end, timezone)} and count once, when first scheduled or published.
      </p>
      {rows(data).map((r) => (
        <Meter key={r.key} row={r} />
      ))}
      <p className="text-sm text-muted-foreground">
        AI agents: {rpm < 0 ? 'no request limit' : `up to ${rpm.toLocaleString()} requests per minute across all your keys and connections`}.
      </p>
    </div>
  );
}
