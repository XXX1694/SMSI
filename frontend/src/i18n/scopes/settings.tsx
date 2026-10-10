'use client';
import type { ReactNode } from 'react';
import { MessagesScope } from '@/i18n/scope';
import settings from '../../../messages/en/settings.json';
import auth from '../../../messages/en/auth.json';

/** /settings: the sign-in methods and password forms read `auth`. The English JSON is a module-level constant, so the scope never re-registers and ships only with this route's chunk. */
const BUNDLES = { settings, auth };

export function SettingsScope({ children }: { children: ReactNode }) {
  return <MessagesScope bundles={BUNDLES}>{children}</MessagesScope>;
}
