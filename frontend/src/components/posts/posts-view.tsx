'use client';
import Link from 'next/link';
import { useRouter, useSearchParams } from 'next/navigation';
import { useCallback, useEffect, useState } from 'react';
import { PostList } from '@/components/post-row';
import { EmptyState, ErrorState, LoadingRows } from '@/components/states';
import { Button } from '@/components/ui/button';
import { Field, Input, Select } from '@/components/ui/input';
import { api } from '@/lib/api';
import { POST_STATUSES, postStatusView } from '@/lib/status';
import type { Post } from '@/lib/types';
import { errorMessage } from '@/hooks';

export function PostsView() {
  const router = useRouter();
  const params = useSearchParams();
  const status = params.get('status') ?? '';
  const from = params.get('from') ?? '';
  const to = params.get('to') ?? '';

  const [items, setItems] = useState<Post[]>([]);
  const [cursor, setCursor] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [more, setMore] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const fetchPage = useCallback(
    async (after?: string) => {
      const toIso = to ? new Date(`${to}T23:59:59`).toISOString() : undefined;
      const fromIso = from ? new Date(`${from}T00:00:00`).toISOString() : undefined;
      return api.posts.list({ status: status || undefined, from: fromIso, to: toIso, limit: 20, cursor: after });
    },
    [status, from, to],
  );

  const reload = useCallback(() => {
    setLoading(true);
    setError(null);
    fetchPage().then(
      (p) => {
        setItems(p.items);
        setCursor(p.next_cursor);
        setLoading(false);
      },
      (e: unknown) => {
        setError(errorMessage(e));
        setLoading(false);
      },
    );
  }, [fetchPage]);

  useEffect(() => {
    reload();
  }, [reload]);

  async function loadMore() {
    if (!cursor) return;
    setMore(true);
    try {
      const p = await fetchPage(cursor);
      setItems((cur) => [...cur, ...p.items]);
      setCursor(p.next_cursor);
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setMore(false);
    }
  }

  function setFilter(key: string, value: string) {
    const next = new URLSearchParams(params.toString());
    if (value) next.set(key, value);
    else next.delete(key);
    router.replace(`/posts${next.toString() ? `?${next}` : ''}`);
  }

  const filtered = Boolean(status || from || to);

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-end gap-3">
        <Field label="Status" htmlFor="f-status">
          <Select id="f-status" value={status} onChange={(e) => setFilter('status', e.target.value)} className="w-48">
            <option value="">All statuses</option>
            {POST_STATUSES.map((s) => (
              <option key={s} value={s}>
                {postStatusView(s).label}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="From" htmlFor="f-from">
          <Input id="f-from" type="date" value={from} onChange={(e) => setFilter('from', e.target.value)} className="w-40" />
        </Field>
        <Field label="To" htmlFor="f-to">
          <Input id="f-to" type="date" value={to} onChange={(e) => setFilter('to', e.target.value)} className="w-40" />
        </Field>
        {filtered ? (
          <Button variant="ghost" size="sm" onClick={() => router.replace('/posts')}>
            Clear filters
          </Button>
        ) : null}
      </div>
      {loading ? (
        <LoadingRows rows={4} />
      ) : error ? (
        <ErrorState error={new Error(error)} onRetry={reload} />
      ) : items.length === 0 ? (
        <EmptyState
          title={filtered ? 'No posts match these filters' : 'No posts yet'}
          action={
            filtered ? undefined : (
              <Button asChild>
                <Link href="/compose">Compose your first post</Link>
              </Button>
            )
          }
        />
      ) : (
        <>
          <PostList posts={items} />
          {cursor ? (
            <div className="text-center">
              <Button variant="secondary" onClick={() => void loadMore()} disabled={more}>
                {more ? 'Loading…' : 'Load more'}
              </Button>
            </div>
          ) : null}
        </>
      )}
    </div>
  );
}
