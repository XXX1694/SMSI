'use client';
import { usePathname } from 'next/navigation';
import { useEffect, useRef } from 'react';

const WAIT_MS = 1000;

/** True when keyboard focus went nowhere: its element was removed by the navigation, or nothing had focus. */
function focusIsLost(): boolean {
  const el = document.activeElement;
  return !el || el === document.body || !el.isConnected;
}

/**
 * After a client-side navigation whose trigger disappeared (signing in, a dialog that navigates, deleting a post),
 * focus falls back to <body> and a keyboard or screen-reader user starts over from the top of the document. This puts
 * it on the page's <main> instead. It never moves focus that is still somewhere (a sidebar link keeps it) and never
 * acts on the first load of the document.
 */
export function FocusOnNavigate() {
  const pathname = usePathname();
  const first = useRef(true);
  useEffect(() => {
    if (first.current) {
      first.current = false;
      return;
    }
    const started = performance.now();
    let frame = 0;
    // The new page may mount a moment later (the app layout shows a loading line first), so look for <main> briefly.
    const tick = () => {
      if (!focusIsLost()) return;
      const main = document.getElementById('main');
      if (main) main.focus({ preventScroll: true });
      else if (performance.now() - started < WAIT_MS) frame = requestAnimationFrame(tick);
    };
    frame = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(frame);
  }, [pathname]);
  return null;
}
