/** Plain-English sentences for API error codes. No raw code (`PROVIDER_ERROR`, `access_denied`) should reach the UI. */
const TEXT: Record<string, string> = {
  VALIDATION_ERROR: 'Some of the details are not valid. Check them and try again.',
  UNAUTHENTICATED: 'You are not signed in, or your session has ended. Sign in again.',
  FORBIDDEN: 'You do not have permission to do this.',
  INSUFFICIENT_SCOPE: 'This API key does not have the permission for that action.',
  QUOTA_EXCEEDED: 'You have reached a limit of your plan. See Settings for what you have used.',
  EMAIL_NOT_VERIFIED: 'Verify your email address first, then try again.',
  NOT_FOUND: 'This item no longer exists. It may have been deleted.',
  INVALID_STATE_TRANSITION: "This post's status changed. Reload to see it.",
  CONFLICT: 'This already exists. Use another name.',
  RATE_LIMITED: 'Too many requests. Wait a moment and try again.',
  SOCIAL_ACCOUNT_EXPIRED: 'This account needs reconnecting. Reconnect it in Accounts.',
  PROVIDER_NOT_AVAILABLE: 'This network is not available yet.',
  PROVIDER_ERROR: "The network rejected the post. See the reason under the post's attempts.",
  INTERNAL: 'Steerpost had a problem. Try again in a moment.',
  NETWORK: 'Cannot reach the server. Check your connection and try again.',
  UNKNOWN: 'Something went wrong. Try again.',
  // OAuth `error` values a network can send back to the callback.
  access_denied: 'The sign-in was canceled, or access was not granted.',
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
