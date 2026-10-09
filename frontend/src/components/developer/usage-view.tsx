'use client';
import { useCallback } from 'react';
import { Sparkline } from '@/components/sparkline';
import { EmptyState, ErrorState, LoadingRows } from '@/components/states';
import { Table, Tbody, Td, Th, Thead, Tr } from '@/components/ui/table';
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
      <Table label="Requests per key">
        <Thead>
          <Tr>
            <Th>Key</Th>
            <Th align="right">Requests</Th>
            <Th>Last used</Th>
          </Tr>
        </Thead>
        <Tbody>
          {data.by_key.map((k) => (
            <Tr key={k.name}>
              <Td label="Key">{k.name}</Td>
              <Td label="Requests" align="right" className="tabular-nums">{k.requests.toLocaleString()}</Td>
              <Td label="Last used" className="text-muted-foreground">{formatRelative(k.last_used_at)}</Td>
            </Tr>
          ))}
        </Tbody>
      </Table>
    </div>
  );
}
