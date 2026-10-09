import { DashboardView } from '@/components/dashboard-view';
import { PageHeader } from '@/components/states';
import { Button } from '@/components/ui/button';
import Link from 'next/link';
import { T } from '@/i18n/t';

export const metadata = { title: 'Dashboard' };

export default function Page() {
  return (
    <>
      <PageHeader
        title={<T k="dashboard.title" />}
        description={<T k="dashboard.subtitle" />}
        actions={
          <Button asChild>
            <Link href="/compose">
              <T k="common.newPost" />
            </Link>
          </Button>
        }
      />
      <DashboardView />
    </>
  );
}
