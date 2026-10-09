'use client';
import { useState } from 'react';
import { ConfirmDialog } from '@/components/confirm-dialog';
import { BASE_PATH, SITE_HREF, wipeDemoStorage } from '@/lib/demo/config';

/**
 * Slim notice shown on every screen of the browser-only demo. It is fixed to the top; the
 * `html[data-demo]` rules in globals.css reserve its height so nothing sits underneath it.
 */
export function DemoBanner() {
  const [confirming, setConfirming] = useState(false);
  // A full page load re-creates the in-memory backend from the (now empty) storage, i.e. a fresh seed.
  async function reset() {
    wipeDemoStorage();
    window.location.assign(`${BASE_PATH}/dashboard/`);
  }

  return (
    <aside
      aria-label="Demo"
      className="fixed inset-x-0 top-0 z-banner flex h-[calc(1.75rem+env(safe-area-inset-top))] items-center justify-between gap-3 border-b bg-muted px-3 pt-[env(safe-area-inset-top)] text-xs text-muted-foreground"
    >
      <p className="flex min-w-0 items-center gap-1">
        <span className="truncate">
          <span className="font-medium text-foreground">Demo</span> — data stays in your browser ·{' '}
        </span>
        {/* The pseudo-element grows the hit area past the 28 px banner without making the banner taller. */}
        <button
          type="button"
          onClick={() => setConfirming(true)}
          className="relative shrink-0 underline underline-offset-2 after:absolute after:-inset-x-2 after:-inset-y-2 after:content-[''] hover:text-foreground"
        >
          Reset
        </button>
      </p>
      <a href={SITE_HREF} className="hidden shrink-0 hover:text-foreground sm:inline">
        About Steerpost
      </a>
      <ConfirmDialog
        open={confirming}
        onOpenChange={setConfirming}
        title="Reset the demo?"
        description="Everything you changed in this demo is removed and the sample data comes back."
        confirmLabel="Reset demo"
        destructive
        onConfirm={reset}
      />
    </aside>
  );
}
