import { SettingsView } from '@/components/settings-view';
import { T } from '@/i18n/t';
import { PageHeader } from '@/components/states';
import { SettingsScope } from '@/i18n/scopes/settings';

export const metadata = { title: 'Settings' };

export default function Page() {
  return (
    <SettingsScope>
      <PageHeader title={<T k="settings.title" />} description={<T k="settings.subtitle" />} />
      <SettingsView />
    </SettingsScope>
  );
}
