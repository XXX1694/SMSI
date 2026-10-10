'use client';
import { usePathname, useRouter } from 'next/navigation';
import { useEffect, type ReactNode } from 'react';
import { useAuth } from '@/components/auth-provider';
import { AppShell } from '@/components/app-shell';
import { ErrorState } from '@/components/states';
import { AppScope } from '@/i18n/scopes/app';
import { useTranslations } from '@/i18n/use-translations';

export default function AppLayout({ children }: { children: ReactNode }) {
  const t = useTranslations('common');
  const { user, loading, error, retry, endedBy } = useAuth();
  const router = useRouter();
  const pathname = usePathname();

  useEffect(() => {
    // After account deletion the login page explains what happened instead of remembering where the user was.
    if (!loading && !user && !error) router.replace(endedBy === 'deleted' ? '/login?deleted=1' : `/login?next=${encodeURIComponent(pathname)}`);
  }, [loading, user, error, router, pathname, endedBy]);

  if (error && !user) {
    return (
      <div className="mx-auto flex min-h-screen max-w-md items-center px-4">
        <div className="w-full">
          <ErrorState error={error} onRetry={retry} />
        </div>
      </div>
    );
  }
  if (loading || !user) {
    return (
      <div role="status" className="flex min-h-screen items-center justify-center text-sm text-muted-foreground">
        {t('loading')}
      </div>
    );
  }
  return (
    <AppScope>
      <AppShell>{children}</AppShell>
    </AppScope>
  );
}
