'use client';
import type { ReactNode } from 'react';
import { MessagesScope } from '@/i18n/scope';
import shell from '../../../messages/en/shell.json';

/** The demo banner, which sits in the root layout of the demo build only. Kept out of CoreScope so the normal build does not ship `shell` on the sign-in pages. The English JSON is a module-level constant, so the scope never re-registers; DemoBannerSlot imports it only in the demo build. */
const BUNDLES = { shell };

export function DemoScope({ children }: { children: ReactNode }) {
  return <MessagesScope bundles={BUNDLES}>{children}</MessagesScope>;
}
