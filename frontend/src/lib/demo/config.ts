/**
 * Demo mode: the whole app runs in the browser against an in-memory mock of the REST API
 * (`npm run build:demo`). Everything here is inert in the normal build.
 *
 * `process.env.NEXT_PUBLIC_*` must be written literally so Next inlines it at build time.
 */
export const DEMO = process.env.NEXT_PUBLIC_DEMO === 'true';

/** Where the static export is served, e.g. `/SMSI/demo` on GitHub Pages. Empty at the domain root. */
export const BASE_PATH = DEMO ? (process.env.NEXT_PUBLIC_BASE_PATH ?? '/SMSI/demo') : '';

/** The marketing/docs site that sits one level above the demo (`/SMSI/demo` -> `/SMSI/`). */
export const SITE_HREF = `${BASE_PATH.replace(/\/demo\/?$/, '').replace(/\/$/, '')}/`;

/** localStorage key of the saved demo state. */
export const STORAGE_KEY = 'socialos_demo_state_v1';

/** Forget the saved demo; the next page load seeds a fresh one. Never throws. */
export function wipeDemoStorage(): void {
  try {
    window.localStorage.removeItem(STORAGE_KEY);
  } catch {
    /* storage blocked: nothing was saved */
  }
}

export const DEMO_EMAIL = 'demo@socialos.dev';
export const DEMO_PASSWORD = 'demo12345';

/** Fired on `window` when the simulated scheduler changed data in the background. */
export const DEMO_CHANGE_EVENT = 'socialos:demo-change';

/** Where a post lives. The demo is a static export, so it cannot serve unknown `/posts/<id>` paths. */
export function postHref(id: string): string {
  return DEMO ? `/posts/view?id=${encodeURIComponent(id)}` : `/posts/${id}`;
}
