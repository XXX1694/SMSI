import { DashboardView } from '@/components/dashboard-view';
import { PageHeader } from '@/components/states';
import { Button } from '@/components/ui/button';
import Link from 'next/link';

export const metadata = { title: 'Dashboard' };

export default function Page() {
  return (
    <>
      <PageHeader
        title="Dashboard"
        description="What is going out, and what needs attention."
        actions={
          <Button asChild>
            <Link href="/compose">New post</Link>
          </Button>
        }
      />
      <DashboardView />
    </>
  );
}
