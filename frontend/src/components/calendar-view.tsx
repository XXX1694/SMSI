'use client';
import { ChevronLeft, ChevronRight } from 'lucide-react';
import Link from 'next/link';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { usePrefs } from '@/components/prefs-provider';
import { useLocaleSettings } from '@/i18n/locale-provider';
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
import { useTranslations } from '@/i18n/use-translations';
import { useFormat } from '@/i18n/use-format';

const TONE_CLASS: Record<string, string> = {
  neutral: 'border-l-muted-foreground/50 bg-muted',
  info: 'border-l-accent bg-accent-soft',
  accent: 'border-l-accent bg-accent-soft',
  success: 'border-l-success bg-success-soft',
  warning: 'border-l-warning bg-warning-soft',
  danger: 'border-l-danger bg-danger-soft',
};

function PostChip({ post, timezone }: { post: Post; timezone: string }) {
  const t = useTranslations();
  const v = postStatusView(post.status, t);
  const time = utcToZonedInputs(postTime(post), timezone).time;
  const title = postLabel(post, t);
  return (
    <Link
      href={postHref(post.id)}
      title={t('calendar.chipTitle', { status: v.label, title })}
      className={cn('relative block min-h-6 truncate rounded-sm border-l-2 px-1.5 py-1 text-xs hover:opacity-80 max-md:py-3.5', TONE_CLASS[v.tone])}
    >
      <span className="tabular-nums text-muted-foreground">{time}</span> {title}
      <span className="sr-only"> ({v.label})</span>
    </Link>
  );
}

function Legend() {
  const t = useTranslations();
  const items = ['draft', 'scheduled', 'published', 'failed'] as const;
  return (
    <ul className="flex flex-wrap gap-3 text-xs text-muted-foreground" aria-label={t('calendar.legend')}>
      {items.map((s) => (
        <li key={s} className="flex items-center gap-1.5">
          <span className={cn('h-3.5 w-4 rounded-[2px] border-l-[3px]', TONE_CLASS[postStatusView(s, t).tone])} aria-hidden />
          {postStatusView(s, t).label}
        </li>
      ))}
    </ul>
  );
}

function DayCell({ day, month, posts, today, timezone, onOpenDay }: { day: string; month: string; posts: Post[]; today: string; timezone: string; onOpenDay: (day: string) => void }) {
  const t = useTranslations('calendar');
  const fmt = useFormat();
  // `day` is a calendar date, not an instant: format it in UTC so a negative offset cannot move it to the day before.
  const dayLabel = fmt.date(day, { timeZone: 'UTC', weekday: 'long', day: 'numeric', month: 'long', year: undefined });
  return (
    <div className={cn('min-h-[5.5rem] border-b border-r p-1.5', day.slice(0, 7) !== month && 'bg-muted/40 text-muted-foreground')}>
      <p className={cn('mb-1 text-xs tabular-nums', day === today && 'inline-flex h-5 w-5 items-center justify-center rounded-full bg-accent font-medium text-accent-foreground')}>
        {dayNumber(day)}
      </p>
      <div className="hidden space-y-1 sm:block">
        {posts.slice(0, 3).map((p) => (
          <PostChip key={p.id} post={p} timezone={timezone} />
        ))}
        {posts.length > 3 ? (
          <button type="button" onClick={() => onOpenDay(day)} className="min-h-6 rounded-sm px-1 text-xs text-muted-foreground underline-offset-4 hover:text-foreground hover:underline">
            <span aria-hidden>{t('more', { count: posts.length - 3 })}</span>
            <span className="sr-only">{t('moreOnDay', { count: posts.length - 3, day: dayLabel })}</span>
          </button>
        ) : null}
      </div>
      {posts.length > 0 ? (
        <button type="button" onClick={() => onOpenDay(day)} className="inline-flex min-h-11 items-center text-xs text-muted-foreground sm:hidden">
          {/* Cells are about 40 px wide at 320 px: only the number fits in languages with long nouns ("publicaciones"); the full text returns once the row has room. */}
          <span aria-hidden className="min-[480px]:hidden">{fmt.number(posts.length)}</span>
          <span aria-hidden className="max-[479px]:hidden">{t('postCount', { count: posts.length })}</span>
          <span className="sr-only">{t('postsOnDay', { count: posts.length, day: dayLabel })}</span>
        </button>
      ) : null}
    </div>
  );
}

function MonthGrid({ anchor, byDay, today, timezone, onOpenDay }: { anchor: string; byDay: Map<string, Post[]>; today: string; timezone: string; onOpenDay: (day: string) => void }) {
  const { locale } = useLocaleSettings();
  const weeks = monthGrid(anchor);
  return (
    <div className="overflow-hidden rounded-lg border-l border-t">
      <div className="grid grid-cols-[repeat(7,minmax(0,1fr))]">
        {(weeks[0] ?? []).map((d) => (
          <div key={d} className="border-b border-r bg-muted/50 px-1.5 py-1 text-xs font-medium text-muted-foreground">
            {weekdayShort(d, locale)}
          </div>
        ))}
        {weeks.flat().map((d) => (
          <DayCell key={d} day={d} month={anchor.slice(0, 7)} posts={byDay.get(d) ?? []} today={today} timezone={timezone} onOpenDay={onOpenDay} />
        ))}
      </div>
    </div>
  );
}

function DayList({ day, posts, timezone, today }: { day: string; posts: Post[]; timezone: string; today: string }) {
  const t = useTranslations('calendar');
  const { locale } = useLocaleSettings();
  return (
    <section aria-label={day}>
      <h3 className={cn('mb-1 text-sm font-medium', day === today && 'text-accent')}>
        {weekdayShort(day, locale)} {dayNumber(day)}
      </h3>
      {posts.length === 0 ? (
        <p className="text-xs text-muted-foreground">{t('noPostsDay')}</p>
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
  const t = useTranslations('calendar');
  const { locale } = useLocaleSettings();
  const { timezone } = usePrefs();
  const todayKey = useMemo(() => dayKey(new Date().toISOString(), timezone), [timezone]);
  const [view, setView] = useState<CalendarView>('month');
  const [anchor, setAnchor] = useState<string | null>(null);
  const [posts, setPosts] = useState<Post[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>(null);
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
      setError(e);
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

  const openDay = (day: string) => {
    setAnchor(day);
    setView('day');
  };

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <Button variant="secondary" size="icon" aria-label={t('previous')} onClick={() => setAnchor(shift(view, current, -1))}>
            <ChevronLeft className="h-4 w-4" aria-hidden />
          </Button>
          <Button variant="secondary" size="icon" aria-label={t('next')} onClick={() => setAnchor(shift(view, current, 1))}>
            <ChevronRight className="h-4 w-4" aria-hidden />
          </Button>
          <Button variant="secondary" size="sm" onClick={() => setAnchor(null)}>
            {t('today')}
          </Button>
          <h2 className="ml-2 text-sm font-semibold" aria-live="polite">
            {titleFor(view, current, locale)}
          </h2>
        </div>
        <div role="group" aria-label={t('viewLabel')} className="inline-flex rounded-md border">
          {(['month', 'week', 'day'] as const).map((v) => (
            <button
              key={v}
              type="button"
              aria-pressed={view === v}
              onClick={() => setView(v)}
              className={cn('px-3 py-1.5 text-sm max-md:min-h-11 first:rounded-l-md last:rounded-r-md', view === v ? 'bg-muted font-medium' : 'text-muted-foreground hover:bg-muted/60')}
            >
              {t(v)}
            </button>
          ))}
        </div>
      </div>
      <Legend />
      {error ? <ErrorState error={error} onRetry={() => void load()} /> : null}
      {loading && posts.length === 0 ? <LoadingRows rows={3} /> : null}
      {!error && view === 'month' ? <MonthGrid anchor={current} byDay={byDay} today={todayKey} timezone={timezone} onOpenDay={openDay} /> : null}
      {!error && view === 'week' ? (
        <div className="grid gap-4 md:grid-cols-7">
          {weekDays(current).map((d) => (
            <DayList key={d} day={d} posts={byDay.get(d) ?? []} timezone={timezone} today={todayKey} />
          ))}
        </div>
      ) : null}
      {!error && view === 'day' ? <DayList day={current} posts={byDay.get(current) ?? []} timezone={timezone} today={todayKey} /> : null}
      {!loading && !error && posts.length === 0 ? <p className="text-sm text-muted-foreground">{t('noPostsPeriod')}</p> : null}
    </div>
  );
}
