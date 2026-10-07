import Link from 'next/link';
import { Suspense } from 'react';
import { PostsView } from '@/components/posts/posts-view';
import { PageHeader } from '@/components/states';
import { Button } from '@/components/ui/button';

export const metadata = { title: 'Posts' };

export default function Page() {
  return (
    <>
      <PageHeader
        title="Posts"
        description="Every draft, scheduled and published post."
        actions={
          <Button asChild>
            <Link href="/compose">New post</Link>
          </Button>
        }
      />
      <Suspense>
        <PostsView />
      </Suspense>
    </>
  );
}
