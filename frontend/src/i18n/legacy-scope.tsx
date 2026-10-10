'use client';
import en from '@/i18n/en-all';
import { MessagesScope } from '@/i18n/scope';
import type { ReactNode } from 'react';

/**
 * The one scope the root layout uses while routes do not declare their own (#145, step 2): every bundle, so the app reads
 * exactly the text it did before. Per-route scopes replace it in step 3. Client module, so the English JSON stays out
 * of the server component payload.
 */
export function LegacyMessagesScope({ children }: { children: ReactNode }) {
  return <MessagesScope bundles={en}>{children}</MessagesScope>;
}
