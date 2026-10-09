import type { AppT } from '@/i18n/translate';
import type { AnalyticsPoint } from './types';

export interface MetricSummary {
  metric: string;
  latest: number;
  total: number;
  series: number[];
}

/** Group points by metric; series sorted by capture time. */
export function summarizeMetrics(points: AnalyticsPoint[]): MetricSummary[] {
  const groups = new Map<string, AnalyticsPoint[]>();
  for (const p of points) groups.set(p.metric, [...(groups.get(p.metric) ?? []), p]);
  return [...groups.entries()]
    .map(([metric, pts]) => {
      const sorted = [...pts].sort((a, b) => a.captured_at.localeCompare(b.captured_at));
      const series = sorted.map((p) => p.value);
      return {
        metric,
        latest: series[series.length - 1] ?? 0,
        total: series.reduce((s, v) => s + v, 0),
        series,
      };
    })
    .sort((a, b) => a.metric.localeCompare(b.metric));
}

const KNOWN_METRICS = ['impressions', 'reactions', 'link_clicks'] as const;

/** A name for a metric id. Metrics a network adds that the catalog does not know yet are shown from their id. */
export function metricLabel(metric: string, t: AppT): string {
  if ((KNOWN_METRICS as readonly string[]).includes(metric)) return t(`analytics.metrics.${metric as (typeof KNOWN_METRICS)[number]}`);
  const s = metric.replace(/[_.]/g, ' ');
  return s.charAt(0).toUpperCase() + s.slice(1);
}
