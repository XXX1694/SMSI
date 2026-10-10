'use client';
import type { ReactNode } from 'react';
import { MessagesScope } from '@/i18n/scope';
import composer from '../../../messages/en/composer.json';
import posts from '../../../messages/en/posts.json';
import media from '../../../messages/en/media.json';

/** /compose: the composer, the post status words and the media picker. The English JSON is a module-level constant, so the scope never re-registers and ships only with this route's chunk. */
const BUNDLES = { composer, posts, media };

export function ComposeScope({ children }: { children: ReactNode }) {
  return <MessagesScope bundles={BUNDLES}>{children}</MessagesScope>;
}
