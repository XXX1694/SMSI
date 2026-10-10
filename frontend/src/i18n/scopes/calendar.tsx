'use client';
import type { ReactNode } from 'react';
import { MessagesScope } from '@/i18n/scope';
import calendar from '../../../messages/en/calendar.json';
import posts from '../../../messages/en/posts.json';

/** /calendar. The English JSON is a module-level constant, so the scope never re-registers and ships only with this route's chunk. */
const BUNDLES = { calendar, posts };

export function CalendarScope({ children }: { children: ReactNode }) {
  return <MessagesScope bundles={BUNDLES}>{children}</MessagesScope>;
}
