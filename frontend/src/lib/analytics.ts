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

export function metricLabel(metric: string): string {
  const s = metric.replace(/[_.]/g, ' ');
  return s.charAt(0).toUpperCase() + s.slice(1);
}
