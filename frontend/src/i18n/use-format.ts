'use client';
import { useMemo } from 'react';
import { usePrefs } from '@/components/prefs-provider';
import { formatDate, formatDateTime, formatNumber, formatTime } from '@/i18n/format';
import { useLocaleSettings } from '@/i18n/locale-provider';

/** Formatters bound to the active locale and the Settings timezone. */
export function useFormat() {
  const { locale } = useLocaleSettings();
  const { timezone } = usePrefs();
  return useMemo(
    () => ({
      number: (n: number, opts?: Intl.NumberFormatOptions) => formatNumber(n, locale, opts),
      date: (iso: string | Date, opts?: Intl.DateTimeFormatOptions) => formatDate(iso, locale, timezone, opts),
      time: (iso: string | Date, opts?: Intl.DateTimeFormatOptions) => formatTime(iso, locale, timezone, opts),
      dateTime: (iso: string | Date, opts?: Intl.DateTimeFormatOptions) => formatDateTime(iso, locale, timezone, opts),
    }),
    [locale, timezone],
  );
}
