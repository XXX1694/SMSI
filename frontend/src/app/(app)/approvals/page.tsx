import { ApprovalsView } from '@/components/approvals/approvals-view';
import { PageHeader } from '@/components/states';

export default function Page() {
  return (
    <>
      <PageHeader
        title="Approvals"
        description="Agents and API keys ask here before they publish now, delete, disconnect an account, connect one or schedule within minutes. Nothing happens until you approve."
      />
      <ApprovalsView />
    </>
  );
}
