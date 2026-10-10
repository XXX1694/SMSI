'use client';
import type { ReactNode } from 'react';
import { MessagesScope } from '@/i18n/scope';
import nav from '../../../messages/en/nav.json';
import shell from '../../../messages/en/shell.json';
import language from '../../../messages/en/language.json';
import legal from '../../../messages/en/legal.json';

/** The signed-in shell (the `(app)` route group): sidebar, header, banners and the footer links. The English JSON is a module-level constant, so the scope never re-registers and ships only with this route's chunk. */
const BUNDLES = { nav, shell, language, legal };

export function AppScope({ children }: { children: ReactNode }) {
  return <MessagesScope bundles={BUNDLES}>{children}</MessagesScope>;
}
