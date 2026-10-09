'use client';
import { usePathname } from 'next/navigation';
import { useEffect, type ReactNode } from 'react';
import { markViewTransitions, notifyNavigated } from '@/lib/view-transition';

/**
 * Wraps the page content. A new pathname remounts the wrapper, which replays the `.page-enter` fade and slide (the
 * fallback); with View Transitions the browser animates the `page-main` snapshot instead and CSS mutes the fallback.
 */
export function PageTransition({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  useEffect(() => markViewTransitions(), []);
  useEffect(() => notifyNavigated(), [pathname]);
  return (
    <div key={pathname} className="page-enter [view-transition-name:page-main]">
      {children}
    </div>
  );
}
