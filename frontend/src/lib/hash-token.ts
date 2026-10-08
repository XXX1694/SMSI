/**
 * Mailed links carry their one-time token in the URL fragment (`/verify-email#token=…`). Browsers never send a
 * fragment to the server or in a Referer header, so the token stays out of access logs. We read it once and strip it
 * from the address bar so it is not left in history, screenshots or a copied URL.
 *
 * The value is kept in memory because React StrictMode (dev) runs effects twice and the second read would find
 * the fragment already gone.
 */
let held: string | null = null;

export function takeHashToken(): string | null {
  if (typeof window === 'undefined') return held;
  const m = /^#token=([^&]+)/.exec(window.location.hash);
  if (m?.[1]) {
    try {
      held = decodeURIComponent(m[1]);
    } catch {
      held = null;
    }
    window.history.replaceState(null, '', window.location.pathname + window.location.search);
  }
  return held;
}

/** Drop the in-memory token once it is spent. */
export function forgetHashToken(): void {
  held = null;
}
