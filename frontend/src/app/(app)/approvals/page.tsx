import { Suspense } from 'react';
import { T } from '@/i18n/t';
import { ApprovalsView } from '@/components/approvals/approvals-view';
import { PageHeader } from '@/components/states';
import { ApprovalsScope } from '@/i18n/scopes/approvals';

export default function Page() {
  return (
    <ApprovalsScope>
      <PageHeader
        title={<T k="approvals.title" />}
        description={<T k="approvals.subtitle" />}
      />
      <Suspense>
        <ApprovalsView />
      </Suspense>
    </ApprovalsScope>
  );
}
