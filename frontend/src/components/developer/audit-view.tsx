'use client';
import { useCallback, useEffect, useState, type ReactNode } from 'react';
import { EmptyState, ErrorState, InlineError, LoadingRows } from '@/components/states';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Table, Tbody, Td, Th, Thead, Tr } from '@/components/ui/table';
import { api } from '@/lib/api';
import type { AuditLog } from '@/lib/types';
import { useErrorText } from '@/hooks';
import type { AppT } from '@/i18n/translate';
import { useTranslations } from '@/i18n/use-translations';
import { useFormat } from '@/i18n/use-format';

/** The audit action the backend writes for every MCP tool call. */
export const TOOL_CALL_ACTION = 'mcp.tool_call';

type Filter = 'all' | 'agents';

interface ToolMeta {
  tool: string;
  status: number | null;
  errorCode: string | null;
  direct: boolean;
}

function toolMeta(l: AuditLog, t: AppT): ToolMeta | null {
  if (l.action !== TOOL_CALL_ACTION) return null;
  const m = l.metadata ?? {};
  return {
    tool: typeof m.tool === 'string' ? m.tool : t('developer.audit.unknownTool'),
    status: typeof m.status === 'number' ? m.status : null,
    errorCode: typeof m.error_code === 'string' ? m.error_code : null,
    direct: m.via_gateway !== true,
  };
}

function ActionCell({ log }: { log: AuditLog }) {
  const t = useTranslations('developer.audit');
  const tc = useTranslations();
  const meta = toolMeta(log, tc);
  if (!meta) return <span className="font-mono text-xs">{log.action}</span>;
  const failed = meta.status !== null && meta.status >= 400;
  return (
    <span className="flex flex-wrap items-center gap-2">
      <span className="font-mono text-xs">{meta.tool}</span>
      {meta.direct ? (
        <Badge tone="warning" title={t('directApiTitle')}>
          {t('directApi')}
        </Badge>
      ) : null}
      {meta.status !== null ? (
        <Badge tone={failed ? 'danger' : 'success'}>
          {meta.status}
          {meta.errorCode ? ` ${meta.errorCode}` : ''}
        </Badge>
      ) : null}
    </span>
  );
}

const FILTERS: readonly Filter[] = ['all', 'agents'];

export function AuditView() {
  const t = useTranslations('developer.audit');
  const tc = useTranslations();
  const fmt = useFormat();
  const errorText = useErrorText();
  const [filter, setFilter] = useState<Filter>('all');
  const [items, setItems] = useState<AuditLog[]>([]);
  const [cursor, setCursor] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>(null);
  const [moreError, setMoreError] = useState<string | null>(null);

  const load = useCallback(async (after?: string) => {
    setLoading(true);
    setError(null);
    setMoreError(null);
    try {
      const p = await api.audit.list(25, after, filter === 'agents' ? TOOL_CALL_ACTION : undefined);
      setItems((cur) => (after ? [...cur, ...p.items] : p.items));
      setCursor(p.next_cursor);
    } catch (e) {
      // A failed next page keeps the rows already shown; only a failed first page replaces the list.
      if (after) setMoreError(errorText(e));
      else setError(e);
    } finally {
      setLoading(false);
    }
  }, [filter, errorText]);

  useEffect(() => {
    setItems([]);
    setCursor(null);
    void load();
  }, [load]);

  let body: ReactNode;
  if (error) body = <ErrorState title={t('loadFailed')} error={error} onRetry={() => void load()} />;
  else if (loading && items.length === 0) body = <LoadingRows rows={3} />;
  else if (items.length === 0) {
    body =
      filter === 'agents' ? (
        <EmptyState title={t('emptyAgentsTitle')}>{t('emptyAgentsBody')}</EmptyState>
      ) : (
        <EmptyState title={t('emptyTitle')} />
      );
  } else body = (
    <div className="space-y-3">
      <Table label={t('tableLabel')}>
        <Thead>
          <Tr>
            <Th>{t('colWhen')}</Th>
            <Th>{t('colActor')}</Th>
            <Th>{t('colAction')}</Th>
            <Th>{t('colResource')}</Th>
          </Tr>
        </Thead>
        <Tbody>
          {items.map((l) => (
            <Tr key={l.id}>
              <Td label={t('colWhen')} className="whitespace-nowrap text-muted-foreground max-md:text-xs">{fmt.dateTime(l.created_at)}</Td>
              <Td label={t('colActor')}>
                <span>
                  <Badge tone={l.actor_type === 'api_key' ? 'accent' : 'neutral'}>{l.actor_type === 'api_key' ? t('actorApiKey') : l.actor_type === 'user' ? t('actorUser') : l.actor_type.replace('_', ' ')}</Badge> {l.actor_label}
                </span>
              </Td>
              <Td label={t('colAction')}>
                <ActionCell log={l} />
              </Td>
              <Td label={t('colResource')} className="text-muted-foreground max-md:text-xs">{l.resource_type}{l.resource_id ? ` ${l.resource_id.slice(0, 8)}` : ''}</Td>
            </Tr>
          ))}
        </Tbody>
      </Table>
      {moreError ? (
        <InlineError onRetry={() => void load(cursor ?? undefined)} retryDisabled={loading} className="rounded-md border border-danger/30 bg-danger-soft px-3 py-2">
          {t('moreFailed', { reason: moreError })}
        </InlineError>
      ) : cursor ? (
        <Button variant="secondary" onClick={() => void load(cursor)} disabled={loading}>
          {loading ? tc('common.loading') : tc('common.loadMore')}
        </Button>
      ) : null}
    </div>
  );
  return (
    <div className="space-y-3">
      <div role="group" aria-label={t('filterLabel')} className="flex flex-wrap gap-2">
        {FILTERS.map((f) => (
          <Button key={f} size="sm" variant={filter === f ? 'primary' : 'secondary'} aria-pressed={filter === f} onClick={() => setFilter(f)}>
            {t(f)}
          </Button>
        ))}
      </div>
      {body}
    </div>
  );
}
