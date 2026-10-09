import type { MetadataRoute } from 'next';
import { BRAND_HEX } from '@/lib/brand';

// Static so the demo export can include it; icons are prefixed with the demo base path when there is one.
export const dynamic = 'force-static';

export default function manifest(): MetadataRoute.Manifest {
  const base = process.env.NEXT_PUBLIC_BASE_PATH ?? '';
  return {
    name: 'Steerpost',
    short_name: 'Steerpost',
    description: 'Self-hosted social publishing: the human steers, the agent posts.',
    start_url: `${base}/`,
    scope: `${base}/`,
    display: 'standalone',
    background_color: BRAND_HEX.light.canvas,
    theme_color: BRAND_HEX.light.canvas, // same as the page theme-color meta; manifests have no light/dark variant
    icons: [
      { src: `${base}/brand/icon-192.png`, sizes: '192x192', type: 'image/png' },
      { src: `${base}/brand/icon-512.png`, sizes: '512x512', type: 'image/png' },
      { src: `${base}/brand/icon-maskable-512.png`, sizes: '512x512', type: 'image/png', purpose: 'maskable' },
    ],
  };
}
