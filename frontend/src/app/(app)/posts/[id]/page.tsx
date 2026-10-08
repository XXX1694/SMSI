import { PostDetail } from '@/components/posts/post-detail';
import { DEMO } from '@/lib/demo/config';
import { SEEDED_POST_IDS } from '@/lib/demo/ids';

export const metadata = { title: 'Post' };

// A static export has to know every dynamic path up front. Only the seeded demo posts get a page
// here (deep links); posts created in the demo open at /posts/view?id=… instead.
// Left undefined in the normal build, where this route stays fully dynamic.
export const generateStaticParams = DEMO ? () => SEEDED_POST_IDS.map((id) => ({ id })) : undefined;

export default async function Page({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <PostDetail id={id} />;
}
