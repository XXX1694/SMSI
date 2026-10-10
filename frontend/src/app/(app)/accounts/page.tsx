import { Suspense } from 'react';
import { T } from '@/i18n/t';
import { AccountsView } from '@/components/accounts-view';
import { PageHeader } from '@/components/states';
import { AccountsScope } from '@/i18n/scopes/accounts';

export const metadata = { title: 'Accounts' };

export default function Page() {
  return (
    <AccountsScope>
      <PageHeader title={<T k="accounts.title" />} description={<T k="accounts.subtitle" />} />
      <Suspense>
        <AccountsView />
      </Suspense>
    </AccountsScope>
  );
}
