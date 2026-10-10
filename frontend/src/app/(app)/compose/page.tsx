import { Suspense } from 'react';
import { LoadingRows } from '@/components/states';
import { ComposeScope } from '@/i18n/scopes/compose';
import { ComposeRoute } from './compose-route';

export const metadata = { title: 'Compose' };

export default function Page() {
  return (
    <ComposeScope>
      <Suspense fallback={<LoadingRows rows={4} />}>
        <ComposeRoute />
      </Suspense>
    </ComposeScope>
  );
}
