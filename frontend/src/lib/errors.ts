import type { AppT } from '@/i18n/translate';
import type en from '../../messages/en.json';

/** Codes with a sentence in the catalog (`errors.<code>`). Anything else gets `errors.UNKNOWN`, never the code itself. */
const CODES = [
  'VALIDATION_ERROR',
  'UNAUTHENTICATED',
  'FORBIDDEN',
  'INSUFFICIENT_SCOPE',
  'QUOTA_EXCEEDED',
  'EMAIL_NOT_VERIFIED',
  'NOT_FOUND',
  'INVALID_STATE_TRANSITION',
  'CONFLICT',
  'RATE_LIMITED',
  'SOCIAL_ACCOUNT_EXPIRED',
  'PROVIDER_NOT_AVAILABLE',
  'PROVIDER_ERROR',
  'INTERNAL',
  'NETWORK',
  'UNKNOWN',
  // OAuth `error` values a network can send back to the callback.
  'access_denied',
] as const satisfies readonly (keyof typeof en.errors)[];
type Code = (typeof CODES)[number];
const isCode = (code: string): code is Code => (CODES as readonly string[]).includes(code);

/** A sentence for an error code. Unknown codes get a generic sentence, never the code itself. */
export function describeErrorCode(code: string | null | undefined, t: AppT): string {
  return t(code && isCode(code) ? `errors.${code}` : 'errors.UNKNOWN');
}

/** True when a server message is a placeholder or a bare code rather than something a person can read. */
export function isTechnicalMessage(message: string | null | undefined): boolean {
  const m = message?.trim();
  if (!m) return true;
  if (/^[A-Z][A-Z0-9_]+$/.test(m) || /^Request failed \(\d+\)\.?$/.test(m)) return true;
  // Raw JSON bodies and stack traces are for logs, not people.
  return /^[{[]/.test(m) || /\n\s+at\s/.test(m) || /\b(?:TypeError|ReferenceError|SyntaxError)\b/.test(m);
}

/** True for English and its pseudo-locale: only then is an English server message fit to show. */
export const isEnglish = (t: AppT): boolean => t.locale === 'en' || t.locale.startsWith('en-');

/**
 * The message to show. In English: the server's own sentence when it is readable, otherwise the sentence for its code.
 * In any other language the server text (English) is never shown: the code's sentence is (D-021).
 */
export function friendlyMessage(code: string | null | undefined, serverMessage: string | null | undefined, t: AppT): string {
  return isEnglish(t) && !isTechnicalMessage(serverMessage) ? (serverMessage as string) : describeErrorCode(code, t);
}
