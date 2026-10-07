/**
 * Pure logic of the Telegram "post a code to prove you own the chat" flow:
 * countdown arithmetic and how the server status and the local clock combine.
 */
import type { TelegramLinkStatus } from './types';

/** The UI asks the server for the link status this often while waiting. */
export const LINK_POLL_INTERVAL_MS = 2000;

/** Below this many seconds the countdown is shown as urgent. */
export const LINK_EXPIRY_WARNING_SECONDS = 60;

export type LinkPhase = 'waiting' | 'connected' | 'expired';

/**
 * Whole seconds until `expiresAt`, rounded up so "0:00" only shows once the code is
 * really gone. Never negative; an unparseable timestamp counts as already expired.
 */
export function secondsLeft(expiresAt: string | number | Date, nowMs: number): number {
  const end = expiresAt instanceof Date ? expiresAt.getTime() : typeof expiresAt === 'number' ? expiresAt : Date.parse(expiresAt);
  if (!Number.isFinite(end) || !Number.isFinite(nowMs)) return 0;
  return Math.max(0, Math.ceil((end - nowMs) / 1000));
}

/** m:ss, e.g. 872 -> "14:32". */
export function formatCountdown(totalSeconds: number): string {
  const s = Math.max(0, Math.floor(Number.isFinite(totalSeconds) ? totalSeconds : 0));
  const mm = Math.floor(s / 60);
  const ss = s % 60;
  return `${mm}:${ss.toString().padStart(2, '0')}`;
}

export function isExpiringSoon(seconds: number): boolean {
  return seconds > 0 && seconds <= LINK_EXPIRY_WARNING_SECONDS;
}

/**
 * What the screen shows. The server is authoritative for "connected" (it wins even
 * when the local countdown already ran out: the user may have posted the code in
 * the last second). A code is expired when the server says so or the countdown is
 * over; before the first poll the server status is unknown (`null`).
 */
export function linkPhase(server: TelegramLinkStatus | null, secondsRemaining: number): LinkPhase {
  if (server === 'connected') return 'connected';
  if (server === 'expired' || secondsRemaining <= 0) return 'expired';
  return 'waiting';
}

/** Only a waiting link needs polling. */
export const shouldPoll = (phase: LinkPhase): boolean => phase === 'waiting';
