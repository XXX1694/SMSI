import { connection } from 'next/server';
import { TermsContent } from '@/components/legal/terms-content';
import { readOperator } from '@/lib/legal';

export const metadata = { title: 'Terms of Service' };

export default async function Page() {
  // The operator's name and contact come from the container's environment, so a real build reads them per request.
  // The demo is a static export and shows the placeholders.
  if (process.env.NEXT_PUBLIC_DEMO !== 'true') await connection();
  return <TermsContent operator={readOperator()} />;
}
