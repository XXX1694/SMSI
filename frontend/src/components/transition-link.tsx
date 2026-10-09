'use client';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import type { ComponentProps, MouseEvent } from 'react';
import { isCurrentPath, runWithTransition } from '@/lib/view-transition';

/**
 * A Link whose plain left-click navigates inside a view transition. Modified clicks (new tab, download, external
 * target) keep the browser's behaviour, so Cmd/Ctrl/middle-click and "open in new tab" still work.
 */
/** The full router target of a Link href, keeping query and hash that an object href carries. */
export function hrefToString(href: ComponentProps<typeof Link>['href']): string {
  if (typeof href === 'string') return href;
  const query = href.query;
  const search = href.search ?? (query ? `?${typeof query === 'string' ? query : new URLSearchParams(query as Record<string, string>).toString()}` : '');
  const hash = href.hash ? (href.hash.startsWith('#') ? href.hash : `#${href.hash}`) : '';
  return `${href.pathname ?? '/'}${search}${hash}`;
}

export function TransitionLink({ onClick, href, target, ...props }: ComponentProps<typeof Link>) {
  const router = useRouter();
  function handleClick(e: MouseEvent<HTMLAnchorElement>) {
    onClick?.(e);
    if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
    if (target && target !== '_self') return;
    const to = hrefToString(href);
    // Same pathname (same page, or only the query/hash changes): no pathname change is signalled, so a transition would
    // only freeze the page until its timeout. Let the Link navigate normally.
    if (isCurrentPath(to)) return;
    e.preventDefault();
    runWithTransition(() => router.push(to));
  }
  return <Link href={href} target={target} onClick={handleClick} {...props} />;
}
