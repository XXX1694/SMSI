'use client';
import { useCallback, useState } from 'react';
import { EmptyState, ErrorState, LoadingRows, Notice } from '@/components/states';
import { Sparkline } from '@/components/sparkline';
import { Select } from '@/components/ui/input';
import { Table, Tbody, Td, Th, Thead, Tr } from '@/components/ui/table';
import { api } from '@/lib/api';
import { metricLabel, summarizeMetrics } from '@/lib/analytics';
import { joinList } from '@/lib/format';
import { useProviderName } from '@/i18n/use-provider-name';
import { useAsync } from '@/hooks';
import { useTranslations } from '@/i18n/use-translations';
import { useFormat } from '@/i18n/use-format';

export function AnalyticsView() {
  const t = useTranslations();
  const providerName = useProviderName();
  const fmt = useFormat();
  const [days, setDays] = useState(30);
  const load = useCallback(async () => {
    const to = new Date();
    const from = new Date(to.getTime() - days * 86_400_000);
    const [analytics, providers] = await Promise.all([api.analytics.get(from.toISOString(), to.toISOString()), api.social.providers()]);
    return { metrics: summarizeMetrics(analytics.items), providers };
  }, [days]);
  const { data, error, loading, reload } = useAsync(load);

  if (loading && !data) return <LoadingRows rows={3} />;
  if (error || !data) return <ErrorState error={error} onRetry={reload} />;
  const withAnalytics = data.providers.filter((p) => p.available && p.capabilities.canAnalytics);

  return (
    <div className="space-y-6">
      <div className="flex items-end justify-between gap-3">
        <label className="space-y-1.5 text-sm font-medium">
          <span>{t('analytics.period')}</span>
          <Select value={days} onChange={(e) => setDays(Number(e.target.value))} className="w-40 font-normal">
            {[7, 30, 90].map((d) => (
              <option key={d} value={d}>
                {t('analytics.lastDays', { days: d })}
              </option>
            ))}
          </Select>
        </label>
      </div>
      {withAnalytics.length === 0 ? (
        <Notice tone="info">{t('analytics.none')}</Notice>
      ) : (
        <p className="text-sm text-muted-foreground">{t('analytics.supportedBy', { networks: joinList(withAnalytics.map((p) => providerName(p.id)), t) })}</p>
      )}
      {data.metrics.length === 0 ? (
        <EmptyState title={t('analytics.emptyTitle')}>{t('analytics.emptyBody')}</EmptyState>
      ) : (
        <Table label={t('analytics.tableLabel')}>
          <Thead>
            <Tr>
              <Th>{t('analytics.colMetric')}</Th>
              <Th align="right">{t('analytics.colLatest')}</Th>
              <Th align="right">{t('analytics.colTotal')}</Th>
              <Th>{t('analytics.colTrend')}</Th>
            </Tr>
          </Thead>
          <Tbody>
            {data.metrics.map((m) => (
              <Tr key={m.metric}>
                <Td label={t('analytics.colMetric')}>{metricLabel(m.metric, t)}</Td>
                <Td label={t('analytics.colLatest')} align="right" className="tabular-nums">{fmt.number(m.latest)}</Td>
                <Td label={t('analytics.colTotal')} align="right" className="tabular-nums">{fmt.number(m.total)}</Td>
                <Td label={t('analytics.colTrend')}><Sparkline values={m.series} label={t('analytics.trend', { metric: metricLabel(m.metric, t) })} /></Td>
              </Tr>
            ))}
          </Tbody>
        </Table>
      )}
    </div>
  );
}
