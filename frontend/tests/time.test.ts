import { describe, expect, it } from 'vitest';
import { dayKey, formatRelative, isValidTimezone, utcToZonedInputs, zonedToUtcIso } from '@/lib/time';

describe('timezone conversion', () => {
  it('converts Almaty wall time (UTC+5) to UTC', () => {
    expect(zonedToUtcIso('2026-11-01', '09:00', 'Asia/Almaty')).toBe('2026-11-01T04:00:00Z');
  });

  it('handles DST: New York summer (UTC-4) vs winter (UTC-5)', () => {
    expect(zonedToUtcIso('2026-07-01', '12:00', 'America/New_York')).toBe('2026-07-01T16:00:00Z');
    expect(zonedToUtcIso('2026-12-01', '12:00', 'America/New_York')).toBe('2026-12-01T17:00:00Z');
  });

  it('crosses date boundaries', () => {
    expect(zonedToUtcIso('2026-11-01', '02:30', 'Asia/Almaty')).toBe('2026-10-31T21:30:00Z');
  });

  it('round-trips through utcToZonedInputs', () => {
    const iso = zonedToUtcIso('2026-03-15', '23:45', 'Europe/Berlin');
    expect(iso).not.toBeNull();
    expect(utcToZonedInputs(iso!, 'Europe/Berlin')).toEqual({ date: '2026-03-15', time: '23:45' });
  });

  it('rejects malformed input and validates zones', () => {
    expect(zonedToUtcIso('2026-1-1', '9:00', 'UTC')).toBeNull();
    expect(isValidTimezone('Mars/Olympus')).toBe(false);
    expect(isValidTimezone('UTC')).toBe(true);
  });

  it('computes day keys in the target zone', () => {
    expect(dayKey('2026-11-01T22:00:00Z', 'Asia/Almaty')).toBe('2026-11-02');
    expect(dayKey('2026-11-01T22:00:00Z', 'UTC')).toBe('2026-11-01');
  });
});

describe('formatRelative', () => {
  const now = new Date('2026-01-01T12:00:00Z');
  it('handles null, past and future', () => {
    expect(formatRelative(null, now)).toBe('Never');
    expect(formatRelative('2026-01-01T10:00:00Z', now)).toBe('2 hours ago');
    expect(formatRelative('2026-01-03T12:00:00Z', now)).toBe('in 2 days');
    expect(formatRelative('2026-01-01T12:00:10Z', now)).toBe('just now');
  });
});
