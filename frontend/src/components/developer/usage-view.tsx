'use client';
import { useCallback } from 'react';
import { Sparkline } from '@/components/sparkline';
import { EmptyState, ErrorState, LoadingRows } from '@/components/states';
import { api } from '@/lib/api';
import { formatRelative } from '@/lib/time';
import { useAsync } from '@/hooks';

export function UsageView() {
  const load = useCallback(() => api.developer.usage(), []);
  const { data, error, loading, reload } = useAsync(load);
  if (loading && !data) return <LoadingRows rows={2} />;
  if (error || !data) return <ErrorState error={error} onRetry={reload} />;
  if (data.total_requests === 0) return <EmptyState title="No API usage yet">Requests made with API keys or MCP connections will be counted here.</EmptyState>;
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-6">
        <div>
          <p className="text-xs text-muted-foreground">Total requests</p>
          <p className="text-2xl font-semibold tabular-nums">{data.total_requests.toLocaleString()}</p>
        </div>
        <Sparkline values={data.by_day.map((d) => d.requests)} label="Requests per day" />
      </div>
      <div className="overflow-x-auto rounded-lg border">
        <table className="w-full text-left text-sm">
          <thead className="border-b bg-muted/50 text-xs text-muted-foreground">
            <tr>
              <th className="px-3 py-2 font-medium">Key</th>
              <th className="px-3 py-2 text-right font-medium">Requests</th>
              <th className="px-3 py-2 font-medium">Last used</th>
            </tr>
          </thead>
          <tbody className="divide-y">
            {data.by_key.map((k) => (
              <tr key={k.name}>
                <td className="px-3 py-2">{k.name}</td>
                <td className="px-3 py-2 text-right tabular-nums">{k.requests.toLocaleString()}</td>
                <td className="px-3 py-2 text-muted-foreground">{formatRelative(k.last_used_at)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
