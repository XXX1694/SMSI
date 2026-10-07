import { Suspense } from 'react';
import { AccountsView } from '@/components/accounts-view';
import { PageHeader } from '@/components/states';

export const metadata = { title: 'Accounts' };

export default function Page() {
  return (
    <>
      <PageHeader title="Accounts" description="Connect the networks you publish to." />
      <Suspense>
        <AccountsView />
      </Suspense>
    </>
  );
}
