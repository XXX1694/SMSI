'use client';
import { usePathname } from 'next/navigation';
import { useEffect, useRef } from 'react';

const WAIT_MS = 1000;

/**
 * True when keyboard focus went nowhere: its element was removed by the navigation, hidden (the mobile menu closes
 * with display:none while its link keeps focus for a moment), or nothing had focus.
 */
function focusIsLost(): boolean {
  const el = document.activeElement;
  return !el || el === document.body || !el.isConnected || el.getClientRects().length === 0;
}

/**
 * After a client-side navigation whose trigger disappeared (signing in, a dialog that navigates, the mobile menu
 * closing), focus falls back to <body> and a keyboard or screen-reader user starts over from the top of the document.
 * This puts it on the page's <main id="main"> instead (the app shell, auth and legal pages all have one). It never
 * moves focus that is still somewhere visible, and never acts on the first load of the document.
 */
export function FocusOnNavigate() {
  const pathname = usePathname();
  // The last path handled: StrictMode runs effects twice on mount, which a "first run" flag would mistake for a navigation.
  const handled = useRef(pathname);
  useEffect(() => {
    if (handled.current === pathname) return;
    handled.current = pathname;
    const started = performance.now();
    let frame = 0;
    // The new page may mount a moment later (the app layout shows a loading line first), so look for it briefly.
    const tick = () => {
      const main = document.getElementById('main');
      if (main && focusIsLost()) main.focus({ preventScroll: true });
      else if (!main && performance.now() - started < WAIT_MS) frame = requestAnimationFrame(tick);
    };
    frame = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(frame);
  }, [pathname]);
  return null;
}
