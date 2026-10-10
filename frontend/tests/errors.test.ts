import { describe, expect, it } from 'vitest';
import { enErrorsT } from '@/i18n/en';
import errors from '../messages/en/errors.json';
import { describeErrorCode, friendlyMessage } from '@/lib/errors';
import { enT } from './helpers/en-t';

describe('describeErrorCode', () => {
  it('turns every backend error code into a sentence without the code in it', () => {
    for (const code of [
      'VALIDATION_ERROR', 'UNAUTHENTICATED', 'FORBIDDEN', 'INSUFFICIENT_SCOPE', 'EMAIL_NOT_VERIFIED', 'NOT_FOUND',
      'INVALID_STATE_TRANSITION', 'CONFLICT', 'RATE_LIMITED', 'SOCIAL_ACCOUNT_EXPIRED', 'PROVIDER_NOT_AVAILABLE',
      'PROVIDER_ERROR', 'INTERNAL', 'NETWORK', 'access_denied',
    ]) {
      const text = describeErrorCode(code, enT);
      expect(text).not.toContain(code);
      expect(text).toMatch(/^[A-Z].*\.$/);
    }
  });

  it('never echoes an unknown code', () => {
    expect(describeErrorCode('SOMETHING_NEW', enT)).toBe('Something went wrong. Try again.');
    expect(describeErrorCode(null, enT)).toBe('Something went wrong. Try again.');
  });
});

describe('friendlyMessage', () => {
  it('keeps a readable server sentence', () => {
    expect(friendlyMessage('UNAUTHENTICATED', 'Invalid email or password.', enT)).toBe('Invalid email or password.');
  });
  it('replaces placeholders and bare codes with the sentence for the code', () => {
    expect(friendlyMessage('INTERNAL', 'Request failed (500).', enT)).toContain('Steerpost had a problem');
    expect(friendlyMessage('PROVIDER_ERROR', 'PROVIDER_ERROR', enT)).not.toContain('PROVIDER_ERROR');
    expect(friendlyMessage('RATE_LIMITED', '', enT)).toContain('Too many requests');
  });
});

describe('technical messages', () => {
  it('hides raw JSON and stack traces behind the sentence for the code', () => {
    expect(friendlyMessage('PROVIDER_ERROR', '{"ok":false,"error_code":400}', enT)).toBe('The network rejected the post. See the reason under the post\'s attempts.');
    expect(friendlyMessage('INTERNAL', 'Error: boom\n    at run (/app/x.js:1:1)', enT)).toContain('Steerpost had a problem');
  });
  it('no longer carries a catch-all "failed" OAuth entry', () => {
    expect(describeErrorCode('failed', enT)).toBe('Something went wrong. Try again.');
  });
});

describe('quota errors', () => {
  it('keeps the readable server message and has a sentence for the bare code', async () => {
    const { friendlyMessage } = await import('@/lib/errors');
    expect(friendlyMessage('QUOTA_EXCEEDED', 'connected accounts limit reached (5 of 5 used).', enT)).toBe('connected accounts limit reached (5 of 5 used).');
    expect(friendlyMessage('QUOTA_EXCEEDED', 'QUOTA_EXCEEDED', enT)).toMatch(/limit of your plan/);
  });
});

describe('enErrorsT (the translator outside React)', () => {
  it('has a sentence for every error code in errors.json', () => {
    const codes = Object.keys(errors).filter((key) => /^[A-Z_]+$|^access_denied$/.test(key));
    expect(codes.length).toBeGreaterThan(10);
    for (const code of codes) {
      expect(describeErrorCode(code, enErrorsT)).toBe(errors[code as keyof typeof errors]);
    }
    // Every other message of the namespace (with its arguments) resolves too, never to its own key.
    for (const key of Object.keys(errors)) {
      expect(enErrorsT(`errors.${key}` as never, { text: 'x', id: 'y' })).not.toContain('errors.');
    }
  });

  it('knows only the errors namespace, so the rest of the English catalog is not shared code', () => {
    expect(enErrorsT('nav.dashboard' as never)).toBe('nav.dashboard');
  });
});
