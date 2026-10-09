'use client';
import { useCallback, useState } from 'react';
import { EmptyState, ErrorState, LoadingRows, Notice } from '@/components/states';
import { Sparkline } from '@/components/sparkline';
import { Select } from '@/components/ui/input';
import { Table, Tbody, Td, Th, Thead, Tr } from '@/components/ui/table';
import { api } from '@/lib/api';
import { metricLabel, summarizeMetrics } from '@/lib/analytics';
import { providerLabel } from '@/lib/normalize';
import { useAsync } from '@/hooks';

export function AnalyticsView() {
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
          <span>Period</span>
          <Select value={days} onChange={(e) => setDays(Number(e.target.value))} className="w-40 font-normal">
            <option value={7}>Last 7 days</option>
            <option value={30}>Last 30 days</option>
            <option value={90}>Last 90 days</option>
          </Select>
        </label>
      </div>
      {withAnalytics.length === 0 ? (
        <Notice tone="info">
          None of the connected networks report analytics yet.
        </Notice>
      ) : (
        <p className="text-sm text-muted-foreground">Analytics supported by: {withAnalytics.map((p) => providerLabel(p.id)).join(', ')}.</p>
      )}
      {data.metrics.length === 0 ? (
        <EmptyState title="No analytics data in this period">Metrics will appear here once a connected network reports them.</EmptyState>
      ) : (
        <Table label="Metrics">
          <Thead>
            <Tr>
              <Th>Metric</Th>
              <Th align="right">Latest</Th>
              <Th align="right">Total</Th>
              <Th>Trend</Th>
            </Tr>
          </Thead>
          <Tbody>
            {data.metrics.map((m) => (
              <Tr key={m.metric}>
                <Td label="Metric">{metricLabel(m.metric)}</Td>
                <Td label="Latest" align="right" className="tabular-nums">{m.latest.toLocaleString()}</Td>
                <Td label="Total" align="right" className="tabular-nums">{m.total.toLocaleString()}</Td>
                <Td label="Trend"><Sparkline values={m.series} label={`${metricLabel(m.metric)} trend`} /></Td>
              </Tr>
            ))}
          </Tbody>
        </Table>
      )}
    </div>
  );
}
