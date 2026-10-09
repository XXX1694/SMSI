import type { ReactNode } from 'react';
import { DeveloperNav } from '@/components/developer/developer-nav';
import { PageHeader } from '@/components/states';

export const metadata = { title: 'Developer' };

export default function DeveloperLayout({ children }: { children: ReactNode }) {
  return (
    <>
      <PageHeader title="Developer" description="API keys, MCP connections, usage and audit log." />
      <DeveloperNav />
      {children}
    </>
  );
}
