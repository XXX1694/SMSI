'use client';
import { ChevronLeft, ChevronRight } from 'lucide-react';
import Link from 'next/link';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { usePrefs } from '@/components/prefs-provider';
import { ErrorState, LoadingRows } from '@/components/states';
import { Button } from '@/components/ui/button';
import { api } from '@/lib/api';
import { postHref } from '@/lib/demo/config';
import { dayNumber, monthGrid, shift, titleFor, visibleRange, weekdayShort, weekDays, type CalendarView } from '@/lib/calendar';
import { postLabel, postTime } from '@/lib/format';
import { postStatusView } from '@/lib/status';
import { dayKey, utcToZonedInputs, zonedToUtcIso } from '@/lib/time';
import type { Post } from '@/lib/types';
import { cn } from '@/lib/utils';
import { errorMessage } from '@/hooks';

const TONE_CLASS: Record<string, string> = {
  neutral: 'border-l-muted-foreground/50 bg-muted',
  info: 'border-l-accent bg-accent-soft',
  accent: 'border-l-accent bg-accent-soft',
  success: 'border-l-success bg-success-soft',
  warning: 'border-l-warning bg-warning-soft',
  danger: 'border-l-danger bg-danger-soft',
};

function PostChip({ post, timezone }: { post: Post; timezone: string }) {
  const v = postStatusView(post.status);
  const time = utcToZonedInputs(postTime(post), timezone).time;
  return (
    <Link
      href={postHref(post.id)}
      title={`${v.label}: ${postLabel(post)}`}
      className={cn('block truncate rounded-sm border-l-2 px-1.5 py-0.5 text-xs hover:opacity-80', TONE_CLASS[v.tone])}
    >
      <span className="tabular-nums text-muted-foreground">{time}</span> {postLabel(post)}
      <span className="sr-only"> ({v.label})</span>
    </Link>
  );
}

function Legend() {
  const items = ['draft', 'scheduled', 'published', 'failed'] as const;
  return (
    <ul className="flex flex-wrap gap-3 text-xs text-muted-foreground" aria-label="Legend">
      {items.map((s) => (
        <li key={s} className="flex items-center gap-1.5">
          <span className={cn('h-3.5 w-4 rounded-[2px] border-l-[3px]', TONE_CLASS[postStatusView(s).tone])} aria-hidden />
          {postStatusView(s).label}
        </li>
      ))}
    </ul>
  );
}

function DayCell({ day, month, posts, today, timezone }: { day: string; month: string; posts: Post[]; today: string; timezone: string }) {
  return (
    <div className={cn('min-h-[5.5rem] border-b border-r p-1.5', day.slice(0, 7) !== month && 'bg-muted/40 text-muted-foreground')}>
      <p className={cn('mb-1 text-xs tabular-nums', day === today && 'inline-flex h-5 w-5 items-center justify-center rounded-full bg-accent font-medium text-accent-foreground')}>
        {dayNumber(day)}
      </p>
      <div className="hidden space-y-0.5 sm:block">
        {posts.slice(0, 3).map((p) => (
          <PostChip key={p.id} post={p} timezone={timezone} />
        ))}
        {posts.length > 3 ? <p className="px-1 text-xs text-muted-foreground">+{posts.length - 3} more</p> : null}
      </div>
      {posts.length > 0 ? <p className="text-xs text-muted-foreground sm:hidden">{posts.length} post{posts.length > 1 ? 's' : ''}</p> : null}
    </div>
  );
}

function MonthGrid({ anchor, byDay, today, timezone }: { anchor: string; byDay: Map<string, Post[]>; today: string; timezone: string }) {
  const weeks = monthGrid(anchor);
  return (
    <div className="overflow-hidden rounded-lg border-l border-t">
      <div className="grid grid-cols-7">
        {(weeks[0] ?? []).map((d) => (
          <div key={d} className="border-b border-r bg-muted/50 px-1.5 py-1 text-xs font-medium text-muted-foreground">
            {weekdayShort(d)}
          </div>
        ))}
        {weeks.flat().map((d) => (
          <DayCell key={d} day={d} month={anchor.slice(0, 7)} posts={byDay.get(d) ?? []} today={today} timezone={timezone} />
        ))}
      </div>
    </div>
  );
}

function DayList({ day, posts, timezone, today }: { day: string; posts: Post[]; timezone: string; today: string }) {
  return (
    <section aria-label={day}>
      <h3 className={cn('mb-1 text-sm font-medium', day === today && 'text-accent')}>
        {weekdayShort(day)} {dayNumber(day)}
      </h3>
      {posts.length === 0 ? (
        <p className="text-xs text-muted-foreground">Nothing</p>
      ) : (
        <div className="space-y-1">
          {posts.map((p) => (
            <PostChip key={p.id} post={p} timezone={timezone} />
          ))}
        </div>
      )}
    </section>
  );
}

export function CalendarViewPage() {
  const { timezone } = usePrefs();
  const todayKey = useMemo(() => dayKey(new Date().toISOString(), timezone), [timezone]);
  const [view, setView] = useState<CalendarView>('month');
  const [anchor, setAnchor] = useState<string | null>(null);
  const [posts, setPosts] = useState<Post[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const current = anchor ?? todayKey;
  const range = useMemo(() => visibleRange(view, current), [view, current]);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const from = zonedToUtcIso(range.start, '00:00', timezone) ?? undefined;
      const to = zonedToUtcIso(range.end, '00:00', timezone) ?? undefined;
      const all: Post[] = [];
      let cursor: string | undefined;
      for (let i = 0; i < 5; i++) {
        const page = await api.posts.list({ from, to, limit: 100, cursor });
        all.push(...page.items);
        if (!page.next_cursor) break;
        cursor = page.next_cursor;
      }
      setPosts(all);
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setLoading(false);
    }
  }, [range, timezone]);

  useEffect(() => {
    void load();
  }, [load]);

  const byDay = useMemo(() => {
    const m = new Map<string, Post[]>();
    for (const p of posts) {
      if (p.status === 'cancelled') continue;
      const k = dayKey(postTime(p), timezone);
      m.set(k, [...(m.get(k) ?? []), p].sort((a, b) => postTime(a).localeCompare(postTime(b))));
    }
    return m;
  }, [posts, timezone]);

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <Button variant="secondary" size="icon" aria-label="Previous" onClick={() => setAnchor(shift(view, current, -1))}>
            <ChevronLeft className="h-4 w-4" aria-hidden />
          </Button>
          <Button variant="secondary" size="icon" aria-label="Next" onClick={() => setAnchor(shift(view, current, 1))}>
            <ChevronRight className="h-4 w-4" aria-hidden />
          </Button>
          <Button variant="secondary" size="sm" onClick={() => setAnchor(null)}>
            Today
          </Button>
          <h2 className="ml-2 text-sm font-semibold" aria-live="polite">
            {titleFor(view, current)}
          </h2>
        </div>
        <div role="group" aria-label="View" className="inline-flex rounded-md border">
          {(['month', 'week', 'day'] as const).map((v) => (
            <button
              key={v}
              type="button"
              aria-pressed={view === v}
              onClick={() => setView(v)}
              className={cn('px-3 py-1.5 text-sm capitalize first:rounded-l-md last:rounded-r-md', view === v ? 'bg-muted font-medium' : 'text-muted-foreground hover:bg-muted/60')}
            >
              {v}
            </button>
          ))}
        </div>
      </div>
      <Legend />
      {error ? <ErrorState error={new Error(error)} onRetry={() => void load()} /> : null}
      {loading && posts.length === 0 ? <LoadingRows rows={3} /> : null}
      {!error && view === 'month' ? <MonthGrid anchor={current} byDay={byDay} today={todayKey} timezone={timezone} /> : null}
      {!error && view === 'week' ? (
        <div className="grid gap-4 md:grid-cols-7">
          {weekDays(current).map((d) => (
            <DayList key={d} day={d} posts={byDay.get(d) ?? []} timezone={timezone} today={todayKey} />
          ))}
        </div>
      ) : null}
      {!error && view === 'day' ? <DayList day={current} posts={byDay.get(current) ?? []} timezone={timezone} today={todayKey} /> : null}
      {!loading && !error && posts.length === 0 ? <p className="text-sm text-muted-foreground">No posts in this period.</p> : null}
    </div>
  );
}
