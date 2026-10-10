'use client';
import type { ReactNode } from 'react';
import { MessagesScope } from '@/i18n/scope';
import posts from '../../../messages/en/posts.json';
import composer from '../../../messages/en/composer.json';

/** /posts/[id] and /posts/view: the editor limits come from `composer`. The English JSON is a module-level constant, so the scope never re-registers and ships only with this route's chunk. */
const BUNDLES = { posts, composer };

export function PostDetailScope({ children }: { children: ReactNode }) {
  return <MessagesScope bundles={BUNDLES}>{children}</MessagesScope>;
}
