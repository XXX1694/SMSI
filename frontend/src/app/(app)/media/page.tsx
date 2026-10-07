import { MediaView } from '@/components/media-view';
import { PageHeader } from '@/components/states';

export const metadata = { title: 'Media' };

export default function Page() {
  return (
    <>
      <PageHeader title="Media" description="Reusable images and videos." />
      <MediaView />
    </>
  );
}
