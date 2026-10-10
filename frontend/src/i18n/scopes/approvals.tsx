'use client';
import type { ReactNode } from 'react';
import { MessagesScope } from '@/i18n/scope';
import approvals from '../../../messages/en/approvals.json';

/** /approvals. The English JSON is a module-level constant, so the scope never re-registers and ships only with this route's chunk. */
const BUNDLES = { approvals };

export function ApprovalsScope({ children }: { children: ReactNode }) {
  return <MessagesScope bundles={BUNDLES}>{children}</MessagesScope>;
}
