import { notFound } from 'next/navigation';
import { Suspense } from 'react';
import { PostByQuery } from '@/components/posts/post-by-query';
import { LoadingRows } from '@/components/states';
import { PostDetailScope } from '@/i18n/scopes/post-detail';
import { DEMO } from '@/lib/demo/config';

export const metadata = { title: 'Post' };

/** Demo build only: a static export cannot serve /posts/<new id>, so the id travels in the query. */
export default function Page() {
  if (!DEMO) notFound();
  return (
    <PostDetailScope>
      <Suspense fallback={<LoadingRows rows={4} />}>
        <PostByQuery />
      </Suspense>
    </PostDetailScope>
  );
}
