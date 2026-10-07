import { CalendarViewPage } from '@/components/calendar-view';
import { PageHeader } from '@/components/states';

export const metadata = { title: 'Calendar' };

export default function Page() {
  return (
    <>
      <PageHeader title="Calendar" description="Drafts, scheduled, published and failed posts in your timezone." />
      <CalendarViewPage />
    </>
  );
}
