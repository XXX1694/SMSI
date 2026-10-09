'use client';
import Link from 'next/link';
import { useRouter, useSearchParams } from 'next/navigation';
import { useCallback, useEffect, useState } from 'react';
import { usePrefs } from '@/components/prefs-provider';
import { PostList } from '@/components/post-row';
import { EmptyState, ErrorState, InlineError, LoadingRows } from '@/components/states';
import { Button } from '@/components/ui/button';
import { Field, Input, Select } from '@/components/ui/input';
import { api } from '@/lib/api';
import { POST_STATUSES, postStatusView } from '@/lib/status';
import { zonedDayRangeIso } from '@/lib/time';
import type { Post } from '@/lib/types';
import { useErrorText } from '@/hooks';
import { useTranslations } from '@/i18n/use-translations';
import { nodes } from '@/i18n/rich';

export function PostsView() {
  const t = useTranslations();
  const tp = useTranslations('posts');
  const errorText = useErrorText();
  const router = useRouter();
  const params = useSearchParams();
  const status = params.get('status') ?? '';
  const from = params.get('from') ?? '';
  const to = params.get('to') ?? '';
  const { timezone } = usePrefs();

  const [items, setItems] = useState<Post[]>([]);
  const [cursor, setCursor] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [more, setMore] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [moreError, setMoreError] = useState<string | null>(null);

  const fetchPage = useCallback(
    async (after?: string) => {
      const range = zonedDayRangeIso(from, to, timezone);
      return api.posts.list({ status: status || undefined, from: range.from, to: range.to, limit: 20, cursor: after });
    },
    [status, from, to, timezone],
  );

  const reload = useCallback(() => {
    setLoading(true);
    setError(null);
    setMoreError(null);
    fetchPage().then(
      (p) => {
        setItems(p.items);
        setCursor(p.next_cursor);
        setLoading(false);
      },
      (e: unknown) => {
        setError(e);
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
    setMoreError(null);
    try {
      const p = await fetchPage(cursor);
      setItems((cur) => [...cur, ...p.items]);
      setCursor(p.next_cursor);
    } catch (e) {
      // Keep the page that is already on screen; only the next page failed.
      setMoreError(errorText(e));
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
        <Field label={tp('filterStatus')} htmlFor="f-status">
          <Select id="f-status" value={status} onChange={(e) => setFilter('status', e.target.value)} className="w-48">
            <option value="">{tp('allStatuses')}</option>
            {POST_STATUSES.map((s) => (
              <option key={s} value={s}>
                {postStatusView(s, t).label}
              </option>
            ))}
          </Select>
        </Field>
        <Field label={tp('filterFrom')} htmlFor="f-from">
          <Input id="f-from" type="date" value={from} onChange={(e) => setFilter('from', e.target.value)} className="w-40" />
        </Field>
        <Field label={tp('filterTo')} htmlFor="f-to">
          <Input id="f-to" type="date" value={to} onChange={(e) => setFilter('to', e.target.value)} className="w-40" />
        </Field>
        {filtered ? (
          <Button variant="ghost" size="sm" onClick={() => router.replace('/posts')}>
            {tp('clearFilters')}
          </Button>
        ) : null}
      </div>
      {loading ? (
        <LoadingRows rows={4} />
      ) : error ? (
        <ErrorState title={tp('loadFailed')} error={error} onRetry={reload} />
      ) : items.length === 0 ? (
        <EmptyState
          title={filtered ? tp('noMatch') : tp('none')}
          action={
            filtered ? undefined : (
              <Button asChild>
                <Link href="/compose">{tp('writePost')}</Link>
              </Button>
            )
          }
        >
          {filtered ? undefined : (
            <>
              {nodes(
                tp.rich('emptyBody', {
                  link: (c) => (
                    <Link href="/accounts" className="underline underline-offset-4">
                      {c}
                    </Link>
                  ),
                }),
              )}
            </>
          )}
        </EmptyState>
      ) : (
        <>
          <PostList posts={items} />
          {moreError ? (
            <InlineError onRetry={() => void loadMore()} retryDisabled={more} className="rounded-md border border-danger/30 bg-danger-soft px-3 py-2">
              {tp('loadMoreFailed', { reason: moreError })}
            </InlineError>
          ) : null}
          {cursor && !moreError ? (
            <div className="text-center">
              <Button variant="secondary" onClick={() => void loadMore()} disabled={more}>
                {more ? t('common.loading') : t('common.loadMore')}
              </Button>
            </div>
          ) : null}
        </>
      )}
    </div>
  );
}
