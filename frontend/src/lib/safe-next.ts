/** Mirrors the API's `redirect.SafePath` (backend/internal/domain/redirect), the one allow-list for in-app redirects. */
const MAX_LEN = 512;
// Backslashes (browsers read them as slashes) and every control character: a tab or newline inside `/\t/host` is
// dropped by the URL parser and turns the path into the protocol-relative `//host`.
// eslint-disable-next-line no-control-regex
const FORBIDDEN = /[\\\u0000-\u001f\u007f]/;

/**
 * The in-app path a sign-in should return to, or null. Only a same-origin absolute path is accepted: a full URL, a
 * protocol-relative `//host`, an embedded scheme or a control character would turn the login page into an open
 * redirect. As a last check the path is resolved against the page's origin and must stay on it.
 */
export function safeNext(raw: string | null | undefined): string | null {
  if (!raw || raw.length > MAX_LEN || !raw.startsWith('/') || raw.startsWith('//')) return null;
  if (FORBIDDEN.test(raw) || raw.includes('://')) return null;
  try {
    const base = typeof window === 'undefined' ? 'http://localhost' : window.location.origin;
    const url = new URL(raw, base);
    // `/.//evil.com` normalises to the path `//evil.com`: same origin here, but protocol-relative wherever it is reused.
    if (url.origin !== new URL(base).origin || url.pathname.startsWith('//')) return null;
  } catch {
    return null;
  }
  return raw;
}

/** `path` with `?next=` appended when there is a safe `next`; the way every link on the auth screens keeps it. */
export function withNext(path: string, next: string | null): string {
  return next ? `${path}?next=${encodeURIComponent(next)}` : path;
}
