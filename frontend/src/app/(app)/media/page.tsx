import { MediaView } from '@/components/media-view';
import { T } from '@/i18n/t';
import { PageHeader } from '@/components/states';
import { MediaScope } from '@/i18n/scopes/media';

export const metadata = { title: 'Media' };

export default function Page() {
  return (
    <MediaScope>
      <PageHeader title={<T k="media.title" />} description={<T k="media.subtitle" />} />
      <MediaView />
    </MediaScope>
  );
}
