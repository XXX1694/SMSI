import Link from 'next/link';
import { T } from '@/i18n/t';
import { Suspense } from 'react';
import { PostsView } from '@/components/posts/posts-view';
import { PageHeader } from '@/components/states';
import { Button } from '@/components/ui/button';
import { PostsScope } from '@/i18n/scopes/posts';

export const metadata = { title: 'Posts' };

export default function Page() {
  return (
    <PostsScope>
      <PageHeader
        title={<T k="posts.title" />}
        description={<T k="posts.subtitle" />}
        actions={
          <Button asChild>
            <Link href="/compose">
              <T k="common.newPost" />
            </Link>
          </Button>
        }
      />
      <Suspense>
        <PostsView />
      </Suspense>
    </PostsScope>
  );
}
