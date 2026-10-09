/** Plain-English sentences for API error codes. No raw code (`PROVIDER_ERROR`, `access_denied`) should reach the UI. */
const TEXT: Record<string, string> = {
  VALIDATION_ERROR: 'Some of the details are not valid. Check them and try again.',
  UNAUTHENTICATED: 'You are not signed in, or your session has ended. Please sign in again.',
  FORBIDDEN: 'You do not have permission to do that.',
  INSUFFICIENT_SCOPE: 'This API key does not have the permission for that action.',
  EMAIL_NOT_VERIFIED: 'Verify your email address first, then try again.',
  NOT_FOUND: 'We could not find that. It may have been deleted.',
  INVALID_STATE_TRANSITION: 'That is not possible for a post in its current status.',
  CONFLICT: 'That conflicts with something that already exists.',
  RATE_LIMITED: 'Too many requests. Please wait a moment and try again.',
  SOCIAL_ACCOUNT_EXPIRED: 'The connection to this account has expired. Reconnect it on the Accounts page.',
  PROVIDER_NOT_AVAILABLE: 'This network is not available yet.',
  PROVIDER_ERROR: 'The network could not publish the post.',
  INTERNAL: 'Something went wrong on our side. Please try again in a moment.',
  NETWORK: 'Cannot reach the server. Check your connection and try again.',
  UNKNOWN: 'Something went wrong. Please try again.',
  // OAuth `error` values a network can send back to the callback.
  access_denied: 'The sign-in was cancelled, or access was not granted.',
};

const GENERIC = TEXT.UNKNOWN as string;

/** A sentence for an error code. Unknown codes get a generic sentence, never the code itself. */
export function describeErrorCode(code: string | null | undefined): string {
  if (!code) return GENERIC;
  return TEXT[code] ?? GENERIC;
}

/** True when a server message is a placeholder or a bare code rather than something a person can read. */
export function isTechnicalMessage(message: string | null | undefined): boolean {
  const m = message?.trim();
  if (!m) return true;
  if (/^[A-Z][A-Z0-9_]+$/.test(m) || /^Request failed \(\d+\)\.?$/.test(m)) return true;
  // Raw JSON bodies and stack traces are for logs, not people.
  return /^[{[]/.test(m) || /\n\s+at\s/.test(m) || /\b(?:TypeError|ReferenceError|SyntaxError)\b/.test(m);
}

/** The message to show: the server's own sentence when it is readable, otherwise the sentence for its code. */
export function friendlyMessage(code: string | null | undefined, serverMessage?: string | null): string {
  return isTechnicalMessage(serverMessage) ? describeErrorCode(code) : (serverMessage as string);
}
