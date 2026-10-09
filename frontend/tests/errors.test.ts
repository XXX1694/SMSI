import { describe, expect, it } from 'vitest';
import { describeErrorCode, friendlyMessage } from '@/lib/errors';

describe('describeErrorCode', () => {
  it('turns every backend error code into a sentence without the code in it', () => {
    for (const code of [
      'VALIDATION_ERROR', 'UNAUTHENTICATED', 'FORBIDDEN', 'INSUFFICIENT_SCOPE', 'EMAIL_NOT_VERIFIED', 'NOT_FOUND',
      'INVALID_STATE_TRANSITION', 'CONFLICT', 'RATE_LIMITED', 'SOCIAL_ACCOUNT_EXPIRED', 'PROVIDER_NOT_AVAILABLE',
      'PROVIDER_ERROR', 'INTERNAL', 'NETWORK', 'access_denied',
    ]) {
      const text = describeErrorCode(code);
      expect(text).not.toContain(code);
      expect(text).toMatch(/^[A-Z].*\.$/);
    }
  });

  it('never echoes an unknown code', () => {
    expect(describeErrorCode('SOMETHING_NEW')).toBe('Something went wrong. Try again.');
    expect(describeErrorCode(null)).toBe('Something went wrong. Try again.');
  });
});

describe('friendlyMessage', () => {
  it('keeps a readable server sentence', () => {
    expect(friendlyMessage('UNAUTHENTICATED', 'Invalid email or password.')).toBe('Invalid email or password.');
  });
  it('replaces placeholders and bare codes with the sentence for the code', () => {
    expect(friendlyMessage('INTERNAL', 'Request failed (500).')).toContain('Steerpost had a problem');
    expect(friendlyMessage('PROVIDER_ERROR', 'PROVIDER_ERROR')).not.toContain('PROVIDER_ERROR');
    expect(friendlyMessage('RATE_LIMITED', '')).toContain('Too many requests');
  });
});

describe('technical messages', () => {
  it('hides raw JSON and stack traces behind the sentence for the code', () => {
    expect(friendlyMessage('PROVIDER_ERROR', '{"ok":false,"error_code":400}')).toBe('The network rejected the post. See the reason under the post\'s attempts.');
    expect(friendlyMessage('INTERNAL', 'Error: boom\n    at run (/app/x.js:1:1)')).toContain('Steerpost had a problem');
  });
  it('no longer carries a catch-all "failed" OAuth entry', () => {
    expect(describeErrorCode('failed')).toBe('Something went wrong. Try again.');
  });
});

describe('quota errors', () => {
  it('keeps the readable server message and has a sentence for the bare code', async () => {
    const { friendlyMessage } = await import('@/lib/errors');
    expect(friendlyMessage('QUOTA_EXCEEDED', 'connected accounts limit reached (5 of 5 used).')).toBe('connected accounts limit reached (5 of 5 used).');
    expect(friendlyMessage('QUOTA_EXCEEDED', 'QUOTA_EXCEEDED')).toMatch(/limit of your plan/);
  });
});
