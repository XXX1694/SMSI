/** Timezone helpers built on Intl only (no date library). */
import { formatRelativeTime } from '@/i18n/format';
import type { AppT } from '@/i18n/translate';
import { addDays } from '@/lib/calendar';

export function browserTimezone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
  } catch {
    return 'UTC';
  }
}

export function isValidTimezone(tz: string): boolean {
  try {
    new Intl.DateTimeFormat('en-US', { timeZone: tz });
    return true;
  } catch {
    return false;
  }
}

export interface Parts {
  year: number;
  month: number;
  day: number;
  hour: number;
  minute: number;
  second: number;
}

export function zonedParts(date: Date, tz: string): Parts {
  const dtf = new Intl.DateTimeFormat('en-US', {
    timeZone: tz,
    hourCycle: 'h23',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  });
  const map: Record<string, number> = {};
  for (const p of dtf.formatToParts(date)) {
    if (p.type !== 'literal') map[p.type] = Number(p.value);
  }
  return {
    year: map.year ?? 1970,
    month: map.month ?? 1,
    day: map.day ?? 1,
    hour: (map.hour ?? 0) % 24,
    minute: map.minute ?? 0,
    second: map.second ?? 0,
  };
}

/** Offset (ms) of `tz` from UTC at the instant `date`. */
function tzOffsetMs(date: Date, tz: string): number {
  const p = zonedParts(date, tz);
  const asUtc = Date.UTC(p.year, p.month - 1, p.day, p.hour, p.minute, p.second);
  return asUtc - Math.floor(date.getTime() / 1000) * 1000;
}

/**
 * Convert a wall-clock date ("YYYY-MM-DD") and time ("HH:mm") in `tz` to a UTC ISO string.
 * Returns null for malformed input.
 */
export function zonedToUtcIso(date: string, time: string, tz: string): string | null {
  const dm = /^(\d{4})-(\d{2})-(\d{2})$/.exec(date);
  const tm = /^(\d{2}):(\d{2})$/.exec(time);
  if (!dm || !tm) return null;
  const [y, mo, d] = [Number(dm[1]), Number(dm[2]), Number(dm[3])];
  const [h, mi] = [Number(tm[1]), Number(tm[2])];
  const guess = Date.UTC(y, mo - 1, d, h, mi, 0);
  // Two passes handle DST boundaries.
  let utc = guess - tzOffsetMs(new Date(guess), tz);
  utc = guess - tzOffsetMs(new Date(utc), tz);
  const result = new Date(utc);
  if (Number.isNaN(result.getTime())) return null;
  return result.toISOString().replace('.000Z', 'Z');
}

/**
 * UTC bounds of whole calendar days ("YYYY-MM-DD") in `tz`: the start of `from` and the last second of `to`.
 * An empty or malformed day gives `undefined` for that bound.
 */
export function zonedDayRangeIso(from: string, to: string, tz: string): { from?: string; to?: string } {
  const start = from ? zonedToUtcIso(from, '00:00', tz) : null;
  let end: string | null = null;
  if (/^\d{4}-\d{2}-\d{2}$/.test(to)) {
    const nextStart = zonedToUtcIso(addDays(to, 1), '00:00', tz);
    if (nextStart) end = new Date(new Date(nextStart).getTime() - 1000).toISOString().replace('.000Z', 'Z');
  }
  return { from: start ?? undefined, to: end ?? undefined };
}

export function utcToZonedInputs(iso: string, tz: string): { date: string; time: string } {
  const p = zonedParts(new Date(iso), tz);
  const pad = (n: number) => String(n).padStart(2, '0');
  return { date: `${p.year}-${pad(p.month)}-${pad(p.day)}`, time: `${pad(p.hour)}:${pad(p.minute)}` };
}

/** "2 hours ago", "yesterday"; `Never` and `just now` come from the catalog. */
export function formatRelative(iso: string | null | undefined, t: AppT, now: Date = new Date()): string {
  if (!iso) return t('common.never');
  return formatRelativeTime(iso, t.locale, now, t('common.justNow'));
}

/** "YYYY-MM-DD" key of an instant in a timezone — used by the calendar. */
export function dayKey(iso: string, tz: string): string {
  return utcToZonedInputs(iso, tz).date;
}
