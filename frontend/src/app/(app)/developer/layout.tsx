import type { ReactNode } from 'react';
import { DeveloperNav } from '@/components/developer/developer-nav';
import { T } from '@/i18n/t';
import { PageHeader } from '@/components/states';
import { DeveloperScope } from '@/i18n/scopes/developer';

export const metadata = { title: 'Developer' };

export default function DeveloperLayout({ children }: { children: ReactNode }) {
  return (
    <DeveloperScope>
      <PageHeader title={<T k="developer.title" />} description={<T k="developer.subtitle" />} />
      <DeveloperNav />
      {children}
    </DeveloperScope>
  );
}
