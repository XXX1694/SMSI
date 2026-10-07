import { ComposerView } from '@/components/composer/composer-view';
import { PageHeader } from '@/components/states';

export const metadata = { title: 'Compose' };

export default function Page() {
  return (
    <>
      <PageHeader title="Compose" description="Write once, tailor per platform, publish or schedule." />
      <ComposerView />
    </>
  );
}
