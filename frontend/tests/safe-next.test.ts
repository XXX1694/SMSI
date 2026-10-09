import { describe, expect, it } from 'vitest';
import { safeNext, withNext } from '@/lib/safe-next';

describe('safeNext', () => {
  it('keeps a percent-encoded tab, which the browser does not decode into a slash', () => {
    expect(safeNext('/%09/evil')).toBe('/%09/evil');
  });

  it.each([
    ['/dashboard', '/dashboard'],
    ['/compose?post=1', '/compose?post=1'],
  ])('keeps the in-app path %s', (raw, want) => expect(safeNext(raw)).toBe(want));

  it.each([null, undefined, '', 'dashboard', 'https://evil.example', '//evil.example', '/\\evil.example', 'javascript:alert(1)', '/\t/evil.example', '/\n/evil.example', '/\r//evil.example', '/.//evil.example', '/a/..//evil.example', '/\u0000/evil', '/\u007f/evil', '/a\tb', '/next?u=https://x.test', `/${'a'.repeat(512)}`])('drops %s', (raw) =>
    expect(safeNext(raw)).toBeNull(),
  );
});

describe('withNext', () => {
  it.each(['/%2F%2Fevil.example', '/%5Cevil.example'])('keeps %s: the browser does not decode it into a slash, so it stays a path here', (raw) =>
    expect(safeNext(raw)).toBe(raw),
  );

  it.each(['\\evil.example', 'https:evil.example'])('drops %s', (raw) => expect(safeNext(raw)).toBeNull());

  it('appends an encoded next, or nothing', () => {
    expect(withNext('/register', '/compose?post=1')).toBe('/register?next=%2Fcompose%3Fpost%3D1');
    expect(withNext('/register', null)).toBe('/register');
  });
});
