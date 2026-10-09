'use client';
import { useSearchParams } from 'next/navigation';
import { ComposerView } from '@/components/composer/composer-view';
import { PageHeader } from '@/components/states';

/** `/compose` writes a new post; `/compose?post=<id>` edits a draft or scheduled one. */
export function ComposeRoute() {
  const postId = useSearchParams().get('post') || undefined;
  return (
    <>
      <PageHeader
        title={postId ? 'Edit post' : 'Compose'}
        description={postId ? 'Change the text, accounts, media or time, then save.' : 'Write once, adjust per network, then schedule or publish.'}
      />
      <ComposerView postId={postId} />
    </>
  );
}
