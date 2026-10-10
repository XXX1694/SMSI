'use client';
import dynamic from 'next/dynamic';

// NEXT_PUBLIC_DEMO is inlined at build time. In the normal build the condition is `false`, so the bundler drops the
// import and neither the banner nor shell.json ships; a static import in the (server) root layout would always be bundled.
const Banner = process.env.NEXT_PUBLIC_DEMO === 'true' ? dynamic(() => import('@/components/demo-banner-scoped')) : null;

/** The demo banner in the demo build; nothing at all in the normal one. */
export function DemoBannerSlot() {
  return Banner ? <Banner /> : null;
}
