import { AnalyticsView } from '@/components/analytics-view';
import { T } from '@/i18n/t';
import { PageHeader } from '@/components/states';

export const metadata = { title: 'Analytics' };

export default function Page() {
  return (
    <>
      <PageHeader title={<T k="analytics.title" />} description={<T k="analytics.subtitle" />} />
      <AnalyticsView />
    </>
  );
}
