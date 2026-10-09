import { Suspense } from 'react';
import { ApprovalsView } from '@/components/approvals/approvals-view';
import { PageHeader } from '@/components/states';

export default function Page() {
  return (
    <>
      <PageHeader
        title="Approvals"
        description="Dangerous actions by agents and API keys wait here. Nothing happens until you approve."
      />
      <Suspense>
        <ApprovalsView />
      </Suspense>
    </>
  );
}
