import * as React from 'react';
import type { GlyphName } from '@/lib/status';
import { cn } from '@/lib/utils';

/**
 * The 12 px shapes that carry a status colour in a tag (D-024, BRAND.md section 5). Each status has its own shape, so the
 * colour is never the only cue; the label next to it says the same in words, so the glyph is hidden from assistive tech.
 */

const RING = <circle cx="6" cy="6" r="4.6" fill="none" stroke="currentColor" strokeWidth="1.4" />;

const SHAPES: Record<GlyphName, React.ReactNode> = {
  draft: <circle cx="6" cy="6" r="4.6" fill="none" stroke="currentColor" strokeWidth="1.4" strokeDasharray="2.1 1.5" />,
  waiting: (
    <>
      {RING}
      <path d="M6 2.6a3.4 3.4 0 0 0 0 6.8z" fill="currentColor" />
    </>
  ),
  scheduled: (
    <>
      {RING}
      <path d="M6 3.5V6l1.7 1.1" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" />
    </>
  ),
  progress: (
    <>
      <circle cx="6" cy="6" r="4.6" fill="none" stroke="currentColor" strokeWidth="1.4" opacity="0.35" />
      <path d="M6 1.4a4.6 4.6 0 0 1 4.6 4.6" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" />
    </>
  ),
  done: <path fillRule="evenodd" d="M6 .9a5.1 5.1 0 1 1 0 10.2A5.1 5.1 0 0 1 6 .9zm2.4 3.3L5.3 7.3 3.7 5.8l-.8.8 2.4 2.3 3.9-3.9z" fill="currentColor" />,
  partial: (
    <>
      {RING}
      <path d="M3.9 6.1l1.4 1.4 2.8-2.8" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round" />
    </>
  ),
  failed: <path fillRule="evenodd" d="M6 .8l5.4 9.6H.6zM5.3 4.3h1.4v3H5.3zm0 3.9h1.4v1.3H5.3z" fill="currentColor" />,
  alert: (
    <>
      {RING}
      <path d="M6 3.4v3.1M6 8.3v.1" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" />
    </>
  ),
  off: (
    <>
      {RING}
      <path d="M2.8 9.2l6.4-6.4" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" />
    </>
  ),
};

export function StatusGlyph({ name, className }: { name: GlyphName; className?: string }) {
  return (
    <svg viewBox="0 0 12 12" width="12" height="12" aria-hidden focusable="false" data-glyph={name} className={cn('shrink-0 forced-colors:text-[CanvasText]', className)}>
      {SHAPES[name]}
    </svg>
  );
}
