'use client';
import { useSearchParams } from 'next/navigation';
import { PostDetail } from '@/components/posts/post-detail';
import { EmptyState } from '@/components/states';

/** `/posts/view?id=…`: the post detail for static (demo) hosting, where ids are not path segments. */
export function PostByQuery() {
  const id = useSearchParams().get('id');
  if (!id) return <EmptyState title="No post selected">Open a post from the Posts page.</EmptyState>;
  return <PostDetail id={id} />;
}
