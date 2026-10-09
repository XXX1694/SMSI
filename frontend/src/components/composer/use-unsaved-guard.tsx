'use client';
import { useRouter } from 'next/navigation';
import { useEffect, useRef, useState } from 'react';
import { ConfirmDialog } from '@/components/confirm-dialog';

function internalTarget(e: MouseEvent): string | null {
  if (e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return null;
  const a = (e.target as Element | null)?.closest?.('a[href]') as HTMLAnchorElement | null;
  if (!a || (a.target && a.target !== '_self') || a.hasAttribute('download')) return null;
  const url = new URL(a.href, window.location.href);
  if (url.origin !== window.location.origin) return null;
  if (url.pathname + url.search === window.location.pathname + window.location.search) return null;
  return url.pathname + url.search + url.hash;
}

/**
 * Warns before unsaved work is lost: the browser's own prompt on reload, close and external links, and a dialog for links
 * inside the app (the App Router has no route-blocking API). Call `allowLeave()` right before navigating after a save.
 */
export function useUnsavedGuard(dirty: boolean) {
  const router = useRouter();
  const [href, setHref] = useState<string | null>(null);
  const allowed = useRef(false);

  useEffect(() => {
    if (!dirty) return undefined;
    const onUnload = (e: BeforeUnloadEvent) => {
      if (allowed.current) return;
      e.preventDefault();
      e.returnValue = '';
    };
    const onClick = (e: MouseEvent) => {
      if (allowed.current || e.defaultPrevented) return;
      const to = internalTarget(e);
      if (!to) return;
      e.preventDefault();
      e.stopPropagation();
      setHref(to);
    };
    window.addEventListener('beforeunload', onUnload);
    document.addEventListener('click', onClick, true);
    return () => {
      window.removeEventListener('beforeunload', onUnload);
      document.removeEventListener('click', onClick, true);
    };
  }, [dirty]);

  const dialog = (
    <ConfirmDialog
      open={href !== null}
      onOpenChange={(o) => !o && setHref(null)}
      title="Discard unsaved changes?"
      description="You have changes that are not saved. If you leave now, they are lost."
      confirmLabel="Discard changes"
      destructive
      onConfirm={async () => {
        allowed.current = true;
        if (href) router.push(href);
      }}
    />
  );
  return {
    dialog,
    allowLeave: () => {
      allowed.current = true;
    },
  };
}
