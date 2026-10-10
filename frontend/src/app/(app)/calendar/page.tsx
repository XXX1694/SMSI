import { CalendarViewPage } from '@/components/calendar-view';
import { T } from '@/i18n/t';
import { PageHeader } from '@/components/states';
import { CalendarScope } from '@/i18n/scopes/calendar';

export const metadata = { title: 'Calendar' };

export default function Page() {
  return (
    <CalendarScope>
      <PageHeader title={<T k="calendar.title" />} description={<T k="calendar.subtitle" />} />
      <CalendarViewPage />
    </CalendarScope>
  );
}
