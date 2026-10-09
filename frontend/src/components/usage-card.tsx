'use client';
import { useCallback } from 'react';
import { ErrorState, LoadingRows } from '@/components/states';
import { api } from '@/lib/api';
import { formatBytes } from '@/lib/media';
import type { QuotaLine, UsageReport } from '@/lib/types';
import { useAsync } from '@/hooks';
import type { AppT } from '@/i18n/translate';
import { useTranslations } from '@/i18n/use-translations';
import { nodes } from '@/i18n/rich';
import { useFormat } from '@/i18n/use-format';

type Row = { key: string; label: string; line: QuotaLine; fmt: (n: number) => string };

function rows(u: UsageReport, t: AppT, number: (n: number) => string): Row[] {
  return [
    { key: 'accounts', label: t('settings.usage.accounts'), line: u.quotas.connected_accounts, fmt: number },
    { key: 'posts', label: t('settings.usage.posts'), line: u.quotas.scheduled_posts_month, fmt: number },
    { key: 'media', label: t('settings.usage.media'), line: u.quotas.media_bytes, fmt: (n) => formatBytes(n, t) },
  ];
}

function Meter({ row }: { row: Row }) {
  const t = useTranslations('settings.usage');
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
          {unlimited
            ? t('lineUnlimited', { used: fmt(used) })
            : t(full ? 'lineFull' : 'line', { used: fmt(used), limit: fmt(line.limit) })}
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
  const t = useTranslations();
  const fmt = useFormat();
  const load = useCallback(() => api.account.usage(), []);
  const { data, error, loading, reload } = useAsync(load);
  if (loading && !data) return <LoadingRows rows={3} />;
  if (error || !data) return <ErrorState error={error} onRetry={reload} title={t('settings.usage.loadFailed')} />;
  const rpm = data.quotas.agent_requests_per_minute.limit;
  return (
    <div className="space-y-4">
      <p className="text-sm text-muted-foreground">
        {nodes(
          t.rich('settings.usage.summary', {
            plan: data.plan,
            start: fmt.dateTime(data.period_start),
            end: fmt.dateTime(data.period_end),
            b: (c) => <span className="font-medium capitalize text-foreground">{c}</span>,
          }),
        )}
      </p>
      {rows(data, t, fmt.number).map((r) => (
        <Meter key={r.key} row={r} />
      ))}
      <p className="text-sm text-muted-foreground">
        {rpm < 0 ? t('settings.usage.agentsUnlimited') : t('settings.usage.agentsLimit', { rpm })}
      </p>
    </div>
  );
}
