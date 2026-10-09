import { describe, expect, it } from 'vitest';
import { formatDate, formatDateTime, formatNumber, formatRelativeTime, formatTime } from '@/i18n/format';
import { dirOf, formatTag } from '@/i18n/locales';

const ISO = '2026-10-09T23:30:00Z';

describe('formatting honours the timezone, not the machine', () => {
  it('shifts the wall clock and the date', () => {
    expect(formatTime(ISO, 'en', 'UTC')).toBe('23:30');
    expect(formatTime(ISO, 'en', 'Asia/Almaty')).toBe('04:30');
    expect(formatDate(ISO, 'en', 'UTC')).toBe('9 Oct 2026');
    expect(formatDate(ISO, 'en', 'Asia/Almaty')).toBe('10 Oct 2026');
  });
  it('en keeps the day-first output the app has always had', () => {
    expect(formatDateTime(ISO, 'en', 'UTC')).toBe('9 Oct 2026, 23:30');
  });
  it('de and ja use their own order', () => {
    expect(formatDate(ISO, 'de', 'UTC', { month: '2-digit' })).toBe('9.10.2026');
    expect(formatDate(ISO, 'ja', 'UTC', { month: '2-digit', day: '2-digit' })).toMatch(/^2026\/10\/09$/);
  });
});

describe('numbers', () => {
  it('uses locale separators', () => {
    expect(formatNumber(1234567.5, 'en')).toBe('1,234,567.5');
    expect(formatNumber(1234567.5, 'de')).toBe('1.234.567,5');
  });
  it('Arabic keeps Latin digits and the Gregorian calendar', () => {
    expect(formatNumber(1234, 'ar')).toBe('1,234');
    expect(formatTag('ar')).toBe('ar-u-nu-latn-ca-gregory');
    expect(formatDate(ISO, 'ar', 'UTC')).toMatch(/2026/);
    expect(formatDate(ISO, 'ar', 'UTC')).not.toMatch(/[٠-٩]/);
  });
});

describe('relative time', () => {
  const now = new Date('2026-10-09T12:00:00Z');
  it('picks the unit and the language', () => {
    expect(formatRelativeTime('2026-10-09T10:00:00Z', 'en', now, 'just now')).toBe('2 hours ago');
    expect(formatRelativeTime('2026-10-09T10:00:00Z', 'de', now, 'gerade eben')).toBe('vor 2 Stunden');
    expect(formatRelativeTime('2026-10-09T12:00:20Z', 'en', now, 'just now')).toBe('just now');
  });
});

describe('dirOf', () => {
  it('is rtl only for Arabic', () => {
    expect(dirOf('ar')).toBe('rtl');
    expect(dirOf('ar-EG')).toBe('rtl');
    for (const l of ['en', 'ru', 'ja', 'zh-CN', 'kk', 'en-XA', 'de']) expect(dirOf(l)).toBe('ltr');
  });
});
