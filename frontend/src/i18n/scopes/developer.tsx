'use client';
import type { ReactNode } from 'react';
import { MessagesScope } from '@/i18n/scope';
import developer from '../../../messages/en/developer.json';

/** /developer, /developer/mcp. The English JSON is a module-level constant, so the scope never re-registers and ships only with this route's chunk. */
const BUNDLES = { developer };

export function DeveloperScope({ children }: { children: ReactNode }) {
  return <MessagesScope bundles={BUNDLES}>{children}</MessagesScope>;
}
