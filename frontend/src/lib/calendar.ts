import { formatTag } from '@/i18n/locales';

/** Pure date-key ("YYYY-MM-DD") arithmetic for the calendar. No timezone logic here. */
export type CalendarView = 'month' | 'week' | 'day';

const pad = (n: number) => String(n).padStart(2, '0');

function toDate(key: string): Date {
  const [y, m, d] = key.split('-').map(Number);
  return new Date(Date.UTC(y ?? 1970, (m ?? 1) - 1, d ?? 1));
}

function toKey(d: Date): string {
  return `${d.getUTCFullYear()}-${pad(d.getUTCMonth() + 1)}-${pad(d.getUTCDate())}`;
}

export function addDays(key: string, n: number): string {
  const d = toDate(key);
  d.setUTCDate(d.getUTCDate() + n);
  return toKey(d);
}

export function addMonths(key: string, n: number): string {
  const d = toDate(key);
  const day = d.getUTCDate();
  d.setUTCDate(1);
  d.setUTCMonth(d.getUTCMonth() + n);
  const last = new Date(Date.UTC(d.getUTCFullYear(), d.getUTCMonth() + 1, 0)).getUTCDate();
  d.setUTCDate(Math.min(day, last));
  return toKey(d);
}

/** Monday of the week containing `key`. */
export function startOfWeek(key: string): string {
  const dow = (toDate(key).getUTCDay() + 6) % 7;
  return addDays(key, -dow);
}

export function weekDays(key: string): string[] {
  const s = startOfWeek(key);
  return Array.from({ length: 7 }, (_, i) => addDays(s, i));
}

/** Full weeks (Mon-start) covering the month of `key`. */
export function monthGrid(key: string): string[][] {
  const first = `${key.slice(0, 7)}-01`;
  const lastDay = new Date(Date.UTC(Number(key.slice(0, 4)), Number(key.slice(5, 7)), 0)).getUTCDate();
  const last = `${key.slice(0, 7)}-${pad(lastDay)}`;
  const weeks: string[][] = [];
  for (let s = startOfWeek(first); s <= last; s = addDays(s, 7)) weeks.push(weekDays(s));
  return weeks;
}

/** Inclusive start, exclusive end (as date keys) of the visible range. */
export function visibleRange(view: CalendarView, key: string): { start: string; end: string } {
  if (view === 'day') return { start: key, end: addDays(key, 1) };
  if (view === 'week') {
    const s = startOfWeek(key);
    return { start: s, end: addDays(s, 7) };
  }
  const grid = monthGrid(key);
  const first = grid[0]?.[0] ?? key;
  const lastWeek = grid[grid.length - 1];
  return { start: first, end: addDays(lastWeek?.[6] ?? key, 1) };
}

export function shift(view: CalendarView, key: string, dir: 1 | -1): string {
  if (view === 'day') return addDays(key, dir);
  if (view === 'week') return addDays(key, 7 * dir);
  return addMonths(key, dir);
}

export function titleFor(view: CalendarView, key: string, locale: string): string {
  const fmt = (k: string, o: Intl.DateTimeFormatOptions) => new Intl.DateTimeFormat(formatTag(locale), { timeZone: 'UTC', ...o }).format(toDate(k));
  if (view === 'month') return fmt(key, { month: 'long', year: 'numeric' });
  if (view === 'day') return fmt(key, { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric' });
  const days = weekDays(key);
  return `${fmt(days[0] ?? key, { day: 'numeric', month: 'short' })} – ${fmt(days[6] ?? key, { day: 'numeric', month: 'short', year: 'numeric' })}`;
}

export function dayNumber(key: string): number {
  return Number(key.slice(8, 10));
}

export function weekdayShort(key: string, locale: string): string {
  return new Intl.DateTimeFormat(formatTag(locale), { timeZone: 'UTC', weekday: 'short' }).format(toDate(key));
}
