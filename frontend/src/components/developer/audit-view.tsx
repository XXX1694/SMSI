'use client';
import { useCallback, useEffect, useState } from 'react';
import { usePrefs } from '@/components/prefs-provider';
import { EmptyState, ErrorState, LoadingRows } from '@/components/states';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { api } from '@/lib/api';
import { formatDateTime } from '@/lib/time';
import type { AuditLog } from '@/lib/types';
import { errorMessage } from '@/hooks';

export function AuditView() {
  const { timezone } = usePrefs();
  const [items, setItems] = useState<AuditLog[]>([]);
  const [cursor, setCursor] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async (after?: string) => {
    setLoading(true);
    setError(null);
    try {
      const p = await api.audit.list(25, after);
      setItems((cur) => (after ? [...cur, ...p.items] : p.items));
      setCursor(p.next_cursor);
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  if (error) return <ErrorState error={new Error(error)} onRetry={() => void load()} />;
  if (loading && items.length === 0) return <LoadingRows rows={3} />;
  if (items.length === 0) return <EmptyState title="No audit events yet" />;
  return (
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
                <td className="px-3 py-2 font-mono text-xs">{l.action}</td>
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
}
