import { connection } from 'next/server';
import { PrivacyContent } from '@/components/legal/privacy-content';
import { readOperator } from '@/lib/legal';

export const metadata = { title: 'Privacy Policy' };

export default async function Page() {
  // The operator's name and contact come from the container's environment, so a real build reads them per request.
  // The demo is a static export and shows the placeholders.
  if (process.env.NEXT_PUBLIC_DEMO !== 'true') await connection();
  return <PrivacyContent operator={readOperator()} />;
}
