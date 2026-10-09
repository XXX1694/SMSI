import { Suspense } from 'react';
import { LoadingRows } from '@/components/states';
import { ComposeRoute } from './compose-route';

export const metadata = { title: 'Compose' };

export default function Page() {
  return (
    <Suspense fallback={<LoadingRows rows={4} />}>
      <ComposeRoute />
    </Suspense>
  );
}
