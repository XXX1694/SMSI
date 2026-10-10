'use client';
import type { ReactNode } from 'react';
import { MessagesScope } from '@/i18n/scope';
import media from '../../../messages/en/media.json';
import composer from '../../../messages/en/composer.json';

/** /media: upload limits and the shared file messages. The English JSON is a module-level constant, so the scope never re-registers and ships only with this route's chunk. */
const BUNDLES = { media, composer };

export function MediaScope({ children }: { children: ReactNode }) {
  return <MessagesScope bundles={BUNDLES}>{children}</MessagesScope>;
}
