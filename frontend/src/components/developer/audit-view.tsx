'use client';
import { useCallback, useEffect, useState, type ReactNode } from 'react';
import { usePrefs } from '@/components/prefs-provider';
import { EmptyState, ErrorState, LoadingRows } from '@/components/states';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { api } from '@/lib/api';
import { formatDateTime } from '@/lib/time';
import type { AuditLog } from '@/lib/types';
import { errorMessage } from '@/hooks';

/** The audit action the backend writes for every MCP tool call. */
export const TOOL_CALL_ACTION = 'mcp.tool_call';

type Filter = 'all' | 'agents';

interface ToolMeta {
  tool: string;
  status: number | null;
  errorCode: string | null;
  direct: boolean;
}

function toolMeta(l: AuditLog): ToolMeta | null {
  if (l.action !== TOOL_CALL_ACTION) return null;
  const m = l.metadata ?? {};
  return {
    tool: typeof m.tool === 'string' ? m.tool : 'unknown tool',
    status: typeof m.status === 'number' ? m.status : null,
    errorCode: typeof m.error_code === 'string' ? m.error_code : null,
    direct: m.via_gateway !== true,
  };
}

function ActionCell({ log }: { log: AuditLog }) {
  const t = toolMeta(log);
  if (!t) return <span className="font-mono text-xs">{log.action}</span>;
  const failed = t.status !== null && t.status >= 400;
  return (
    <span className="flex flex-wrap items-center gap-2">
      <span className="font-mono text-xs">{t.tool}</span>
      {t.direct ? (
        <Badge tone="warning" title="Sent straight to the API, not through the MCP server">
          Direct API
        </Badge>
      ) : null}
      {t.status !== null ? (
        <Badge tone={failed ? 'danger' : 'success'}>
          {t.status}
          {t.errorCode ? ` ${t.errorCode}` : ''}
        </Badge>
      ) : null}
    </span>
  );
}

const FILTERS: readonly { id: Filter; label: string }[] = [
  { id: 'all', label: 'All activity' },
  { id: 'agents', label: 'Agent actions' },
];

export function AuditView() {
  const { timezone } = usePrefs();
  const [filter, setFilter] = useState<Filter>('all');
  const [items, setItems] = useState<AuditLog[]>([]);
  const [cursor, setCursor] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async (after?: string) => {
    setLoading(true);
    setError(null);
    try {
      const p = await api.audit.list(25, after, filter === 'agents' ? TOOL_CALL_ACTION : undefined);
      setItems((cur) => (after ? [...cur, ...p.items] : p.items));
      setCursor(p.next_cursor);
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setLoading(false);
    }
  }, [filter]);

  useEffect(() => {
    setItems([]);
    setCursor(null);
    void load();
  }, [load]);

  let body: ReactNode;
  if (error) body = <ErrorState error={new Error(error)} onRetry={() => void load()} />;
  else if (loading && items.length === 0) body = <LoadingRows rows={3} />;
  else if (items.length === 0) {
    body =
      filter === 'agents' ? (
        <EmptyState title="No agent actions yet">Tool calls made through MCP appear here.</EmptyState>
      ) : (
        <EmptyState title="No audit events yet" />
      );
  } else body = (
    <div className="space-y-3">
      <div className="overflow-x-auto rounded-lg border">
        <table className="w-full text-left text-sm">
          <thead className="border-b bg-muted/50 text-xs text-muted-foreground">
            <tr>
              <th className="px-3 py-2 font-medium">When</th>
              <th className="px-3 py-2 font-medium">Actor</th>
              <th className="px-3 py-2 font-medium">Action</th>
              <th className="px-3 py-2 font-medium">Resource</th>
            </tr>
          </thead>
          <tbody className="divide-y">
            {items.map((l) => (
              <tr key={l.id}>
                <td className="whitespace-nowrap px-3 py-2 text-muted-foreground">{formatDateTime(l.created_at, timezone)}</td>
                <td className="px-3 py-2">
                  <Badge tone={l.actor_type === 'api_key' ? 'accent' : 'neutral'}>{l.actor_type.replace('_', ' ')}</Badge> {l.actor_label}
                </td>
                <td className="px-3 py-2">
                  <ActionCell log={l} />
                </td>
                <td className="px-3 py-2 text-muted-foreground">{l.resource_type}{l.resource_id ? ` ${l.resource_id.slice(0, 8)}` : ''}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {cursor ? (
        <Button variant="secondary" onClick={() => void load(cursor)} disabled={loading}>
          {loading ? 'Loading…' : 'Load more'}
        </Button>
      ) : null}
    </div>
  );
  return (
    <div className="space-y-3">
      <div role="group" aria-label="Filter audit events" className="flex gap-2">
        {FILTERS.map((f) => (
          <Button key={f.id} size="sm" variant={filter === f.id ? 'primary' : 'secondary'} aria-pressed={filter === f.id} onClick={() => setFilter(f.id)}>
            {f.label}
          </Button>
        ))}
      </div>
      {body}
    </div>
  );
}
