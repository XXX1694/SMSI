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
    expect(describeErrorCode('SOMETHING_NEW')).toBe('Something went wrong. Please try again.');
    expect(describeErrorCode(null)).toBe('Something went wrong. Please try again.');
  });
});

describe('friendlyMessage', () => {
  it('keeps a readable server sentence', () => {
    expect(friendlyMessage('UNAUTHENTICATED', 'Invalid email or password.')).toBe('Invalid email or password.');
  });
  it('replaces placeholders and bare codes with the sentence for the code', () => {
    expect(friendlyMessage('INTERNAL', 'Request failed (500).')).toContain('our side');
    expect(friendlyMessage('PROVIDER_ERROR', 'PROVIDER_ERROR')).not.toContain('PROVIDER_ERROR');
    expect(friendlyMessage('RATE_LIMITED', '')).toContain('Too many requests');
  });
});
