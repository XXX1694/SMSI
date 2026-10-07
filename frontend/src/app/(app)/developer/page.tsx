import { ApiKeysView } from '@/components/developer/api-keys-view';
import { AuditView } from '@/components/developer/audit-view';
import { UsageView } from '@/components/developer/usage-view';
import { Section } from '@/components/states';

export default function Page() {
  return (
    <div className="space-y-12">
      <Section title="API keys">
        <ApiKeysView />
      </Section>
      <Section title="Usage">
        <UsageView />
      </Section>
      <Section title="Audit log">
        <AuditView />
      </Section>
    </div>
  );
}
