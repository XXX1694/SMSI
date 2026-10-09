/**
 * The two hero transitions of D-024 (BRAND §7) that need no shared element: an accent circle that fills the screen from
 * the pressed button when signing in, and a diagonal accent wipe when publishing now. Both are one fixed overlay that
 * never takes the pointer, at most --duration-hero long, animated with clip-path and opacity only. Reduced motion or
 * Pause motion skips them: the action simply happens.
 */
import { prefersReducedMotion } from '@/lib/view-transition';

type Point = { x: number; y: number };

function token(name: string, fallback: string): string {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim() || fallback;
}

function heroMs(): number {
  const raw = token('--duration-hero', '520ms');
  const ms = raw.endsWith('ms') ? parseFloat(raw) : raw.endsWith('s') ? parseFloat(raw) * 1000 : NaN;
  return Number.isFinite(ms) ? ms : 520;
}

function overlay(): HTMLDivElement {
  const el = document.createElement('div');
  el.className = 'hero-overlay';
  el.setAttribute('aria-hidden', 'true');
  document.body.appendChild(el);
  return el;
}

/** Resolves once the address bar shows a different path, or after `limit` ms (a navigation that never commits). */
function pathChanged(from: string, limit: number): Promise<void> {
  return new Promise((resolve) => {
    const started = performance.now();
    const tick = () => (window.location.pathname !== from || performance.now() - started > limit ? resolve() : window.setTimeout(tick, 16));
    tick();
  });
}

/** Center of an element, for the circle to start from; the middle of the screen when there is none. */
export function centerOf(el: Element | null | undefined): Point {
  if (!el) return { x: window.innerWidth / 2, y: window.innerHeight / 2 };
  const r = el.getBoundingClientRect();
  return { x: r.left + r.width / 2, y: r.top + r.height / 2 };
}

/**
 * Sign-in: the accent circle grows from `from` while `navigate` runs, then fades to reveal the new screen once its path
 * has committed. `navigate` always runs exactly once, at once.
 */
export function heroFill(from: Point, navigate: () => void): void {
  if (prefersReducedMotion() || typeof document.body.animate !== 'function') {
    navigate();
    return;
  }
  const half = heroMs() / 2;
  const ease = token('--ease-fill', 'ease-in-out');
  const el = overlay();
  const at = `at ${Math.round(from.x)}px ${Math.round(from.y)}px`;
  const reach = Math.hypot(Math.max(from.x, window.innerWidth - from.x), Math.max(from.y, window.innerHeight - from.y));
  const startPath = window.location.pathname;
  navigate();
  const circle = (radius: number) => `circle(${radius}px ${at})`;
  const grow = el.animate([{ clipPath: circle(0) }, { clipPath: circle(Math.ceil(reach)) }], { duration: half, easing: ease, fill: 'forwards' });
  void grow.finished
    .then(() => pathChanged(startPath, 300))
    .then(() => el.animate([{ opacity: 1 }, { opacity: 0 }], { duration: half, easing: ease, fill: 'forwards' }).finished)
    .catch(() => undefined)
    .finally(() => el.remove());
}

/** Publish now: a diagonal accent band crosses the screen. Resolves when it has passed, so success lands after it. */
export async function heroWipe(): Promise<void> {
  if (prefersReducedMotion() || typeof document.body.animate !== 'function') return;
  const el = overlay();
  const frames = [
    { clipPath: 'polygon(-40% 0, -10% 0, -40% 100%, -70% 100%)' },
    { clipPath: 'polygon(20% 0, 70% 0, 40% 100%, -10% 100%)', offset: 0.5 },
    { clipPath: 'polygon(140% 0, 170% 0, 140% 100%, 110% 100%)' },
  ];
  try {
    await el.animate(frames, { duration: heroMs(), easing: token('--ease-fill', 'ease-in-out'), fill: 'forwards' }).finished;
  } catch {
    /* cancelled (the page went away): nothing to finish */
  } finally {
    el.remove();
  }
}
