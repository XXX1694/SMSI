import { describe, expect, it } from 'vitest';
import { addDays, addMonths, monthGrid, shift, startOfWeek, visibleRange, weekDays } from '@/lib/calendar';
import { formatBytes, validateMediaFile } from '@/lib/media';
import { summarizeMetrics } from '@/lib/analytics';
import { enT } from './helpers/en-t';

describe('calendar math', () => {
  it('starts weeks on Monday', () => {
    expect(startOfWeek('2026-10-07')).toBe('2026-10-05');
    expect(weekDays('2026-10-11')[0]).toBe('2026-10-05');
  });
  it('adds days and months with clamping', () => {
    expect(addDays('2026-12-31', 1)).toBe('2027-01-01');
    expect(addMonths('2026-01-31', 1)).toBe('2026-02-28');
    expect(shift('month', '2026-03-15', -1)).toBe('2026-02-15');
    expect(shift('week', '2026-03-15', 1)).toBe('2026-03-22');
  });
  it('builds full-week month grids', () => {
    const g = monthGrid('2026-10-15');
    expect(g.every((w) => w.length === 7)).toBe(true);
    expect(g[0]?.[0]).toBe('2026-09-28');
    expect(g.flat()).toContain('2026-10-31');
  });
  it('computes visible ranges', () => {
    expect(visibleRange('day', '2026-10-15')).toEqual({ start: '2026-10-15', end: '2026-10-16' });
    expect(visibleRange('week', '2026-10-15')).toEqual({ start: '2026-10-12', end: '2026-10-19' });
  });
});

describe('media validation', () => {
  it('accepts allowed types within limits', () => {
    expect(validateMediaFile({ name: 'a.png', type: 'image/png', size: 1000 }, enT)).toBeNull();
    expect(validateMediaFile({ name: 'a.mp4', type: 'video/mp4', size: 50 * 1024 * 1024 }, enT)).toBeNull();
  });
  it('rejects wrong types and oversize files', () => {
    expect(validateMediaFile({ name: 'a.pdf', type: 'application/pdf', size: 1 }, enT)).toMatch(/unsupported/);
    expect(validateMediaFile({ name: 'big.jpg', type: 'image/jpeg', size: 11 * 1024 * 1024 }, enT)).toMatch(/too large/);
    expect(validateMediaFile({ name: 'big.mov', type: 'video/quicktime', size: 101 * 1024 * 1024 }, enT)).toMatch(/too large/);
    expect(validateMediaFile({ name: 'e.png', type: 'image/png', size: 0 }, enT)).toMatch(/empty/);
  });
  it('formats bytes', () => {
    expect(formatBytes(512, enT)).toBe('512 B');
    expect(formatBytes(2048, enT)).toBe('2 KB');
    expect(formatBytes(5 * 1024 * 1024, enT)).toBe('5.0 MB');
  });
});

describe('analytics summary', () => {
  it('groups by metric sorted by time', () => {
    const s = summarizeMetrics([
      { metric: 'views', value: 5, captured_at: '2026-01-02' },
      { metric: 'views', value: 3, captured_at: '2026-01-01' },
    ]);
    expect(s).toEqual([{ metric: 'views', latest: 5, total: 8, series: [3, 5] }]);
  });
});
