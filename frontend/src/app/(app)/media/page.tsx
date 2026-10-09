import { MediaView } from '@/components/media-view';
import { T } from '@/i18n/t';
import { PageHeader } from '@/components/states';

export const metadata = { title: 'Media' };

export default function Page() {
  return (
    <>
      <PageHeader title={<T k="media.title" />} description={<T k="media.subtitle" />} />
      <MediaView />
    </>
  );
}
