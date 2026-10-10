'use client';
import type { ReactNode } from 'react';
import { MessagesScope } from '@/i18n/scope';
import auth from '../../../messages/en/auth.json';
import legal from '../../../messages/en/legal.json';

/** Sign-in, sign-up, verification and password reset screens (the `(auth)` route group). The English JSON is a module-level constant, so the scope never re-registers and ships only with this route's chunk. */
const BUNDLES = { auth, legal };

export function AuthScope({ children }: { children: ReactNode }) {
  return <MessagesScope bundles={BUNDLES}>{children}</MessagesScope>;
}
