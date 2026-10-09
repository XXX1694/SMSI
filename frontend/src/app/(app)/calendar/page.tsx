import { CalendarViewPage } from '@/components/calendar-view';
import { T } from '@/i18n/t';
import { PageHeader } from '@/components/states';

export const metadata = { title: 'Calendar' };

export default function Page() {
  return (
    <>
      <PageHeader title={<T k="calendar.title" />} description={<T k="calendar.subtitle" />} />
      <CalendarViewPage />
    </>
  );
}
