import type { ReactNode } from 'react';
import { AuthScope } from '@/i18n/scopes/auth';

/** Sign-in, sign-up, verification and password reset share one set of messages (`auth`, `legal`); the root has the rest. */
export default function AuthLayout({ children }: { children: ReactNode }) {
  return <AuthScope>{children}</AuthScope>;
}
