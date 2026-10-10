import { ApiKeysView } from '@/components/developer/api-keys-view';
import { AuditView } from '@/components/developer/audit-view';
import { UsageView } from '@/components/developer/usage-view';
import { Section } from '@/components/ui/card';
import { AnalyticsScope } from '@/i18n/scopes/analytics';
import { T } from '@/i18n/t';

export default function Page() {
  return (
    <AnalyticsScope>
      <div className="space-y-12">
        <Section title={<T k="developer.keysSection" />}>
          <ApiKeysView />
        </Section>
        <Section title={<T k="developer.usageSection" />}>
          <UsageView />
        </Section>
        <Section title={<T k="developer.auditSection" />}>
          <AuditView />
        </Section>
      </div>
    </AnalyticsScope>
  );
}
