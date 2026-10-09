'use client';
import { DataExportCard } from '@/components/data-export';
import { DeleteAccount } from '@/components/delete-account';
import { Section } from '@/components/ui/card';
import { useTranslations } from '@/i18n/use-translations';

/** Settings, "Your data": export first, then the account deletion block (D-018, D-019). */
export function YourData() {
  const t = useTranslations('settings');
  return (
    <Section title={t('yourData')}>
      <div className="space-y-8">
        <div className="space-y-2">
          <h3 className="text-sm font-medium">{t('exportHeading')}</h3>
          <DataExportCard />
        </div>
        <div className="space-y-2">
          <h3 className="text-sm font-medium text-danger">{t('deleteAccount.heading')}</h3>
          <DeleteAccount />
        </div>
      </div>
    </Section>
  );
}
