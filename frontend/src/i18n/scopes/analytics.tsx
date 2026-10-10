'use client';
import type { ReactNode } from 'react';
import { MessagesScope } from '@/i18n/scope';
import analytics from '../../../messages/en/analytics.json';

/** /analytics, and the usage sparkline on /developer. The English JSON is a module-level constant, so the scope never re-registers and ships only with this route's chunk. */
const BUNDLES = { analytics };

export function AnalyticsScope({ children }: { children: ReactNode }) {
  return <MessagesScope bundles={BUNDLES}>{children}</MessagesScope>;
}
