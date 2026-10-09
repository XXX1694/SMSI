import { describe, expect, it } from 'vitest';
import { safeNext, withNext } from '@/lib/safe-next';

describe('safeNext', () => {
  it.each([
    ['/dashboard', '/dashboard'],
    ['/compose?post=1', '/compose?post=1'],
  ])('keeps the in-app path %s', (raw, want) => expect(safeNext(raw)).toBe(want));

  it.each([null, undefined, '', 'dashboard', 'https://evil.example', '//evil.example', '/\\evil.example', 'javascript:alert(1)'])('drops %s', (raw) =>
    expect(safeNext(raw)).toBeNull(),
  );
});

describe('withNext', () => {
  it('appends an encoded next, or nothing', () => {
    expect(withNext('/register', '/compose?post=1')).toBe('/register?next=%2Fcompose%3Fpost%3D1');
    expect(withNext('/register', null)).toBe('/register');
  });
});
