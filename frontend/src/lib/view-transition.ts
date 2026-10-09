/**
 * Route transitions via the View Transitions API, with a plain fallback.
 *
 * Next.js navigations are asynchronous: `router.push` returns before the new page is committed. The browser needs a
 * promise that settles when the DOM has changed, so `runWithTransition` waits until `notifyNavigated` is called (the
 * layout calls it when the pathname changes) or a timeout passes. Where the API is missing, or the user asked for
 * reduced motion, the navigation just runs and CSS (`.page-enter`) provides the entry animation.
 */
type StartViewTransition = (update: () => Promise<void>) => unknown;

const TIMEOUT_MS = 1000;
let pending: (() => void) | null = null;

export function prefersReducedMotion(): boolean {
  return typeof window !== 'undefined' && typeof window.matchMedia === 'function' && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
}

export function supportsViewTransitions(): boolean {
  return typeof document !== 'undefined' && typeof (document as Document & { startViewTransition?: unknown }).startViewTransition === 'function';
}

function stripSlashes(path: string): string {
  const trimmed = path.replace(/\/+$/, '');
  return trimmed === '' ? '/' : trimmed;
}

/**
 * True when `target` (a router href) has the pathname the browser is already on. Query-only and hash-only changes keep
 * the pathname, which is the only thing that signals a committed navigation (`notifyNavigated`), so they must not
 * start a view transition: it would wait for the timeout.
 */
export function isCurrentPath(target: string): boolean {
  if (typeof window === 'undefined') return false;
  const base = (process.env.NEXT_PUBLIC_BASE_PATH ?? '').replace(/\/+$/, '');
  const url = new URL(target, window.location.origin);
  const here = window.location;
  const herePath = base && here.pathname.startsWith(base) ? here.pathname.slice(base.length) : here.pathname;
  return stripSlashes(url.pathname) === stripSlashes(herePath);
}

/** Marks <html> so CSS can drop the fallback animation in browsers that animate through the API. */
export function markViewTransitions(): void {
  if (supportsViewTransitions() && !prefersReducedMotion()) document.documentElement.dataset.vt = '';
}

/** Called when a navigation has been committed; lets the pending transition capture the new page. */
export function notifyNavigated(): void {
  const done = pending;
  pending = null;
  done?.();
}

/** Runs `navigate` inside a view transition when possible. Always runs `navigate` exactly once. Returns true if animated. */
export function runWithTransition(navigate: () => void): boolean {
  if (!supportsViewTransitions() || prefersReducedMotion()) {
    navigate();
    return false;
  }
  const start = (document as Document & { startViewTransition: StartViewTransition }).startViewTransition.bind(document);
  start(
    () =>
      new Promise<void>((resolve) => {
        const timer = window.setTimeout(resolve, TIMEOUT_MS);
        pending = () => {
          window.clearTimeout(timer);
          // Not requestAnimationFrame: rendering is suspended while the update callback is pending, so a frame never comes.
          // The DOM is already committed here (React ran the effect), so a task tick is enough.
          window.setTimeout(resolve, 0);
        };
        navigate();
      }),
  );
  return true;
}
