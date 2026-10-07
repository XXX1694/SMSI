import { SettingsView } from '@/components/settings-view';
import { PageHeader } from '@/components/states';

export const metadata = { title: 'Settings' };

export default function Page() {
  return (
    <>
      <PageHeader title="Settings" description="Your profile and preferences." />
      <SettingsView />
    </>
  );
}
