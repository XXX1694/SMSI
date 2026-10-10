'use client';
import type { ReactNode } from 'react';
import { MessagesScope } from '@/i18n/scope';
import dashboard from '../../../messages/en/dashboard.json';
import posts from '../../../messages/en/posts.json';

/** /dashboard. The English JSON is a module-level constant, so the scope never re-registers and ships only with this route's chunk. */
const BUNDLES = { dashboard, posts };

export function DashboardScope({ children }: { children: ReactNode }) {
  return <MessagesScope bundles={BUNDLES}>{children}</MessagesScope>;
}
