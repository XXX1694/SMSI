'use client';
import { useRouter } from 'next/navigation';
import { useEffect, useRef, useState } from 'react';
import { ConfirmDialog } from '@/components/confirm-dialog';
import { useTranslations } from '@/i18n/use-translations';

function internalTarget(e: MouseEvent): string | null {
  if (e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return null;
  const a = (e.target as Element | null)?.closest?.('a[href]') as HTMLAnchorElement | null;
  if (!a || (a.target && a.target !== '_self') || a.hasAttribute('download')) return null;
  const url = new URL(a.href, window.location.href);
  if (url.origin !== window.location.origin) return null;
  if (url.pathname + url.search === window.location.pathname + window.location.search) return null;
  // `a.href` carries the deploy base path (the Pages demo); router.push adds it again, so hand it a path without it.
  const base = (process.env.NEXT_PUBLIC_BASE_PATH ?? '').replace(/\/+$/, '');
  const path = base && (url.pathname === base || url.pathname.startsWith(`${base}/`)) ? url.pathname.slice(base.length) || '/' : url.pathname;
  return path + url.search + url.hash;
}

/**
 * Warns before unsaved work is lost: the browser's own prompt on reload, close and external links, and a dialog for links
 * inside the app (the App Router has no route-blocking API). Call `allowLeave()` right before navigating after a save.
 */
export function useUnsavedGuard(dirty: boolean) {
  const t = useTranslations('composer');
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
      title={t('discardTitle')}
      description={t('discardBody')}
      confirmLabel={t('discardConfirm')}
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
