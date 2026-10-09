/**
 * The in-app path a sign-in should return to. Only a same-origin absolute path is accepted: a full URL, a
 * protocol-relative `//host` or a backslash trick would turn the login page into an open redirect. The server checks
 * it again (`SafeRedirectPath`); this keeps the links we build clean.
 */
export function safeNext(raw: string | null | undefined): string | null {
  if (!raw || !raw.startsWith('/') || raw.startsWith('//') || raw.includes('\\')) return null;
  return raw;
}

/** `path` with `?next=` appended when there is a safe `next`; the way every link on the auth screens keeps it. */
export function withNext(path: string, next: string | null): string {
  return next ? `${path}?next=${encodeURIComponent(next)}` : path;
}
