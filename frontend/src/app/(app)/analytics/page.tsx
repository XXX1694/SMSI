import { AnalyticsView } from '@/components/analytics-view';
import { T } from '@/i18n/t';
import { PageHeader } from '@/components/states';
import { AnalyticsScope } from '@/i18n/scopes/analytics';

export const metadata = { title: 'Analytics' };

export default function Page() {
  return (
    <AnalyticsScope>
      <PageHeader title={<T k="analytics.title" />} description={<T k="analytics.subtitle" />} />
      <AnalyticsView />
    </AnalyticsScope>
  );
}
