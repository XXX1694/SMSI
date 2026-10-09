'use client';
import { useRouter } from 'next/navigation';
import { useCallback, useState, type MouseEvent } from 'react';
import { ProviderMark } from '@/components/provider-icons';
import { InlineError } from '@/components/states';
import { buttonVariants } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { useAsync, useErrorText } from '@/hooks';
import { useTranslations } from '@/i18n/use-translations';
import { api } from '@/lib/api';
import { DEMO } from '@/lib/demo/config';
import type { SignInProvider } from '@/lib/types';
import { cn } from '@/lib/utils';

/** The providers this server has switched on. `error` is set when the list could not be loaded: not the same as "none". */
export function useSignInProviders() {
  const load = useCallback(() => api.auth.signInProviders(), []);
  const { data, error, loading, reload } = useAsync(load);
  return { providers: data ?? [], error, loading, reload };
}

/**
 * One full-width button per provider. In the real build each is a link to the API's `/start` route (a full-page
 * navigation to the provider). The demo has no provider to visit, so it asks the mock where the redirect would end.
 */
export function ProviderButtons({ providers, next, intent }: { providers: SignInProvider[]; next: string | null; intent: 'continue' | 'again' }) {
  const t = useTranslations('auth');
  const errorText = useErrorText();
  const router = useRouter();
  const [failure, setFailure] = useState<string | null>(null);

  async function demoStart(e: MouseEvent, provider: string) {
    e.preventDefault();
    setFailure(null);
    try {
      router.push((await api.auth.startSocialDemo(provider, next)).redirect);
    } catch (err) {
      setFailure(errorText(err));
    }
  }

  return (
    <div className="space-y-2">
      {providers.map((p) => (
        <a
          key={p.id}
          href={api.auth.socialStartUrl(p.id, next)}
          onClick={DEMO ? (e) => void demoStart(e, p.id) : undefined}
          className={cn(buttonVariants({ variant: 'secondary' }), 'w-full')}
        >
          <ProviderMark id={p.id} className="h-4 w-4" />
          {t(intent === 'continue' ? 'continueWith' : 'signInAgainWith', { provider: p.name })}
        </a>
      ))}
      {failure ? <InlineError>{failure}</InlineError> : null}
    </div>
  );
}

/** The "Continue with Google / GitHub" block of the sign-in and sign-up screens, plus the divider above the email form. */
export function SocialSignIn({ next }: { next: string | null }) {
  const t = useTranslations('auth');
  const { providers, error, loading, reload } = useSignInProviders();

  if (loading) {
    return (
      <div role="status" className="mt-6 space-y-2">
        <Skeleton className="h-9 w-full max-md:h-11" />
        <Skeleton className="h-9 w-full max-md:h-11" />
        <span className="sr-only">{t('providersLoading')}</span>
      </div>
    );
  }
  // A failed load is not "no providers": say so, keep the email form usable.
  if (error) {
    return (
      <InlineError className="mt-6" onRetry={reload}>
        {t('providersFailed')}
      </InlineError>
    );
  }
  if (providers.length === 0) return null;
  return (
    <div className="mt-6">
      <ProviderButtons providers={providers} next={next} intent="continue" />
      <div className="mt-6 flex items-center gap-3 text-xs text-muted-foreground" role="separator" aria-label={t('orDivider')}>
        <span className="h-px flex-1 bg-border" aria-hidden />
        <span aria-hidden>{t('orDivider')}</span>
        <span className="h-px flex-1 bg-border" aria-hidden />
      </div>
    </div>
  );
}
