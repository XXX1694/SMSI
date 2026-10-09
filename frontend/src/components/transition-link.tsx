'use client';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import type { ComponentProps, MouseEvent } from 'react';
import { runWithTransition } from '@/lib/view-transition';

/**
 * A Link whose plain left-click navigates inside a view transition. Modified clicks (new tab, download, external
 * target) keep the browser's behaviour, so Cmd/Ctrl/middle-click and "open in new tab" still work.
 */
export function TransitionLink({ onClick, href, target, ...props }: ComponentProps<typeof Link>) {
  const router = useRouter();
  function handleClick(e: MouseEvent<HTMLAnchorElement>) {
    onClick?.(e);
    if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
    if (target && target !== '_self') return;
    e.preventDefault();
    runWithTransition(() => router.push(typeof href === 'string' ? href : (href.pathname ?? '/')));
  }
  return <Link href={href} target={target} onClick={handleClick} {...props} />;
}
