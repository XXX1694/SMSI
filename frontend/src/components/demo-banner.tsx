'use client';
import { BASE_PATH, SITE_HREF, wipeDemoStorage } from '@/lib/demo/config';

/**
 * Slim notice shown on every screen of the browser-only demo. It is fixed to the top; the
 * `html[data-demo]` rules in globals.css reserve its height so nothing sits underneath it.
 */
export function DemoBanner() {
  // A full page load re-creates the in-memory backend from the (now empty) storage, i.e. a fresh seed.
  function reset() {
    wipeDemoStorage();
    window.location.assign(`${BASE_PATH}/dashboard/`);
  }

  return (
    <div
      role="note"
      className="fixed inset-x-0 top-0 z-banner flex h-7 items-center justify-between gap-3 border-b bg-muted px-3 text-xs text-muted-foreground"
    >
      <p className="truncate">
        <span className="font-medium text-foreground">Demo</span> — data stays in your browser ·{' '}
        <button type="button" onClick={reset} className="underline underline-offset-2 hover:text-foreground">
          Reset
        </button>
      </p>
      <a href={SITE_HREF} className="hidden shrink-0 hover:text-foreground sm:inline">
        About SocialOS
      </a>
    </div>
  );
}
