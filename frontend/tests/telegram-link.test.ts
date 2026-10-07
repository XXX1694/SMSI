import { describe, expect, it } from 'vitest';
import {
  LINK_EXPIRY_WARNING_SECONDS,
  LINK_POLL_INTERVAL_MS,
  formatCountdown,
  isExpiringSoon,
  linkPhase,
  secondsLeft,
  shouldPoll,
} from '@/lib/telegram-link';
import { normalizeTelegramLink, normalizeTelegramLinkState } from '@/lib/normalize';

const T0 = Date.parse('2026-10-07T12:00:00.000Z');

describe('secondsLeft', () => {
  const expires = '2026-10-07T12:15:00.000Z';

  it('counts down to the server expiry', () => {
    expect(secondsLeft(expires, T0)).toBe(900);
    expect(secondsLeft(expires, T0 + 60_000)).toBe(840);
    expect(secondsLeft(expires, T0 + 899_000)).toBe(1);
  });

  it('rounds up so 0 only appears when the code is really gone', () => {
    expect(secondsLeft(expires, T0 + 899_001)).toBe(1);
    expect(secondsLeft(expires, T0 + 899_999)).toBe(1);
    expect(secondsLeft(expires, T0 + 900_000)).toBe(0);
  });

  it('never goes negative', () => {
    expect(secondsLeft(expires, T0 + 3_600_000)).toBe(0);
  });

  it('accepts Date and epoch inputs', () => {
    expect(secondsLeft(new Date(expires), T0)).toBe(900);
    expect(secondsLeft(Date.parse(expires), T0)).toBe(900);
  });

  it('treats garbage as already expired', () => {
    expect(secondsLeft('', T0)).toBe(0);
    expect(secondsLeft('tomorrow-ish', T0)).toBe(0);
    expect(secondsLeft(expires, Number.NaN)).toBe(0);
  });
});

describe('formatCountdown', () => {
  it('formats m:ss', () => {
    expect(formatCountdown(900)).toBe('15:00');
    expect(formatCountdown(872)).toBe('14:32');
    expect(formatCountdown(61)).toBe('1:01');
    expect(formatCountdown(9)).toBe('0:09');
    expect(formatCountdown(0)).toBe('0:00');
  });

  it('is safe for bad input', () => {
    expect(formatCountdown(-5)).toBe('0:00');
    expect(formatCountdown(Number.NaN)).toBe('0:00');
    expect(formatCountdown(59.9)).toBe('0:59');
  });
});

describe('isExpiringSoon', () => {
  it('is true in the last minute, false before and once it is over', () => {
    expect(isExpiringSoon(LINK_EXPIRY_WARNING_SECONDS + 1)).toBe(false);
    expect(isExpiringSoon(LINK_EXPIRY_WARNING_SECONDS)).toBe(true);
    expect(isExpiringSoon(1)).toBe(true);
    expect(isExpiringSoon(0)).toBe(false);
  });
});

describe('linkPhase', () => {
  it('waits while the server has not seen the code and time is left', () => {
    expect(linkPhase(null, 900)).toBe('waiting');
    expect(linkPhase('pending', 5)).toBe('waiting');
  });

  it('is expired when the server says so, even with time left on a skewed local clock', () => {
    expect(linkPhase('expired', 600)).toBe('expired');
  });

  it('is expired when the local countdown is over and the server has not said otherwise', () => {
    expect(linkPhase('pending', 0)).toBe('expired');
    expect(linkPhase(null, 0)).toBe('expired');
  });

  it('lets a connection win over the local countdown (posted in the last second)', () => {
    expect(linkPhase('connected', 0)).toBe('connected');
    expect(linkPhase('connected', 500)).toBe('connected');
  });
});

describe('polling policy', () => {
  it('polls every 2 seconds, and only while waiting', () => {
    expect(LINK_POLL_INTERVAL_MS).toBe(2000);
    expect(shouldPoll('waiting')).toBe(true);
    expect(shouldPoll('connected')).toBe(false);
    expect(shouldPoll('expired')).toBe(false);
  });
});

describe('response normalisation', () => {
  it('reads the link start response', () => {
    expect(
      normalizeTelegramLink({ id: 'l1', code: 'SOS-7KQ2M9XA', expires_at: '2026-10-07T12:15:00Z', bot_username: '@socialos_bot', instructions: 'do it' }),
    ).toEqual({ id: 'l1', code: 'SOS-7KQ2M9XA', expires_at: '2026-10-07T12:15:00Z', bot_username: 'socialos_bot', instructions: 'do it' });
    expect(normalizeTelegramLink(null)).toEqual({ id: '', code: '', expires_at: '', bot_username: '', instructions: '' });
  });

  it('reads the status response and never invents a connection', () => {
    const account = { id: 'a1', provider: 'telegram', username: 'chan', display_name: 'Chan' };
    expect(normalizeTelegramLinkState({ status: 'connected', account })).toEqual({ status: 'connected', account });
    expect(normalizeTelegramLinkState({ status: 'pending' })).toEqual({ status: 'pending', account: null });
    expect(normalizeTelegramLinkState({ status: 'expired' }).status).toBe('expired');
    expect(normalizeTelegramLinkState({ status: 'something-new' }).status).toBe('pending');
    expect(normalizeTelegramLinkState(undefined)).toEqual({ status: 'pending', account: null });
  });
});
