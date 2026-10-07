import { AnalyticsView } from '@/components/analytics-view';
import { PageHeader } from '@/components/states';

export const metadata = { title: 'Analytics' };

export default function Page() {
  return (
    <>
      <PageHeader title="Analytics" description="Performance reported by your platforms." />
      <AnalyticsView />
    </>
  );
}
