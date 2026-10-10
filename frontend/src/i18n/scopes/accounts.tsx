'use client';
import type { ReactNode } from 'react';
import { MessagesScope } from '@/i18n/scope';
import accounts from '../../../messages/en/accounts.json';

/** /accounts. The English JSON is a module-level constant, so the scope never re-registers and ships only with this route's chunk. */
const BUNDLES = { accounts };

export function AccountsScope({ children }: { children: ReactNode }) {
  return <MessagesScope bundles={BUNDLES}>{children}</MessagesScope>;
}
