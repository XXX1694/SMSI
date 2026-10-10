'use client';
import type { ReactNode } from 'react';
import { MessagesScope } from '@/i18n/scope';
import common from '../../../messages/en/common.json';
import errors from '../../../messages/en/errors.json';

/** Every route: the shared words (`common`) and the sentences for API error codes (`errors`), which hooks and dialogs read anywhere. The English JSON is a module-level constant, so the scope never re-registers and ships only with this route's chunk. */
const BUNDLES = { common, errors };

export function CoreScope({ children }: { children: ReactNode }) {
  return <MessagesScope bundles={BUNDLES}>{children}</MessagesScope>;
}
