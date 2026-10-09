'use client';
import { useCallback } from 'react';
import { Sparkline } from '@/components/sparkline';
import { EmptyState, ErrorState, LoadingRows } from '@/components/states';
import { Table, Tbody, Td, Th, Thead, Tr } from '@/components/ui/table';
import { api } from '@/lib/api';
import { formatRelative } from '@/lib/time';
import { useAsync } from '@/hooks';
import { useTranslations } from '@/i18n/use-translations';
import { useFormat } from '@/i18n/use-format';

export function UsageView() {
  const t = useTranslations();
  const fmt = useFormat();
  const load = useCallback(() => api.developer.usage(), []);
  const { data, error, loading, reload } = useAsync(load);
  if (loading && !data) return <LoadingRows rows={2} />;
  if (error || !data) return <ErrorState error={error} onRetry={reload} />;
  if (data.total_requests === 0) return <EmptyState title={t('developer.usage.noneTitle')}>{t('developer.usage.noneBody')}</EmptyState>;
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-6">
        <div>
          <p className="text-xs text-muted-foreground">{t('developer.usage.totalRequests')}</p>
          <p className="text-2xl font-semibold tabular-nums">{fmt.number(data.total_requests)}</p>
        </div>
        <Sparkline values={data.by_day.map((d) => d.requests)} label={t('developer.usage.perDay')} />
      </div>
      <Table label={t('developer.usage.tableLabel')}>
        <Thead>
          <Tr>
            <Th>{t('developer.usage.colKey')}</Th>
            <Th align="right">{t('developer.usage.colRequests')}</Th>
            <Th>{t('developer.usage.colLastUsed')}</Th>
          </Tr>
        </Thead>
        <Tbody>
          {data.by_key.map((k) => (
            <Tr key={k.name}>
              <Td label={t('developer.usage.colKey')}>{k.name}</Td>
              <Td label={t('developer.usage.colRequests')} align="right" className="tabular-nums">{fmt.number(k.requests)}</Td>
              <Td label={t('developer.usage.colLastUsed')} className="text-muted-foreground">{formatRelative(k.last_used_at, t)}</Td>
            </Tr>
          ))}
        </Tbody>
      </Table>
    </div>
  );
}
