/**
 * Intl-based formatting that honours the locale and the Settings timezone. These are the only formatters the UI should
 * use; `lib/time.ts` and `lib/calendar.ts` still hard-code `en-GB` until the formatting PR moves them over.
 */
import { formatTag } from '@/i18n/locales';

export function formatNumber(n: number, locale: string, opts?: Intl.NumberFormatOptions): string {
  return new Intl.NumberFormat(formatTag(locale), opts).format(n);
}

export function formatDate(iso: string | Date, locale: string, tz: string, opts?: Intl.DateTimeFormatOptions): string {
  return new Intl.DateTimeFormat(formatTag(locale), {
    timeZone: tz,
    day: 'numeric',
    month: 'short',
    year: 'numeric',
    ...opts,
  }).format(new Date(iso));
}

export function formatTime(iso: string | Date, locale: string, tz: string, opts?: Intl.DateTimeFormatOptions): string {
  return new Intl.DateTimeFormat(formatTag(locale), {
    timeZone: tz,
    hour: '2-digit',
    minute: '2-digit',
    hourCycle: 'h23',
    ...opts,
  }).format(new Date(iso));
}

export function formatDateTime(iso: string | Date, locale: string, tz: string, opts?: Intl.DateTimeFormatOptions): string {
  return new Intl.DateTimeFormat(formatTag(locale), {
    timeZone: tz,
    day: 'numeric',
    month: 'short',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    hourCycle: 'h23',
    ...opts,
  }).format(new Date(iso));
}

const UNITS: [Intl.RelativeTimeFormatUnit, number][] = [
  ['day', 86_400_000],
  ['hour', 3_600_000],
  ['minute', 60_000],
];

/** "in 2 hours", "yesterday". Under a minute gives `justNow`, which the caller supplies translated. */
export function formatRelativeTime(iso: string | Date, locale: string, now: Date, justNow: string): string {
  const diff = new Date(iso).getTime() - now.getTime();
  const abs = Math.abs(diff);
  const rtf = new Intl.RelativeTimeFormat(formatTag(locale), { numeric: 'auto' });
  for (const [unit, ms] of UNITS) {
    if (abs >= ms) return rtf.format(Math.round(diff / ms), unit);
  }
  return justNow;
}
