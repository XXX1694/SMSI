'use client';
import { InlineError, Notice } from '@/components/states';
import { ProviderButtons, useSignInProviders } from '@/components/social-sign-in';
import { useErrorText } from '@/hooks';
import { useTranslations } from '@/i18n/use-translations';

/**
 * A session too old for a sensitive change by an account without a password: sign in again with a provider and come back
 * to Settings. The provider list loads only now, not on every visit. `linked` limits the buttons to the providers the
 * user can actually use; with none given, every enabled provider is offered.
 */
export function ReauthNotice({ body, linked }: { body: string; linked: readonly string[] }) {
  const t = useTranslations();
  const errorText = useErrorText();
  const { providers, error, loading, reload } = useSignInProviders();
  const mine = providers.filter((p) => linked.includes(p.id));
  return (
    <Notice tone="warning">
      <p>{body}</p>
      <div className="mt-3">
        {loading ? <p role="status">{t('common.loading')}</p> : null}
        {error ? <InlineError onRetry={reload}>{errorText(error)}</InlineError> : null}
        <ProviderButtons providers={mine.length > 0 ? mine : providers} next="/settings" intent="again" />
      </div>
    </Notice>
  );
}

/** The provider ids among `/me` login methods (everything except "password"). */
export function linkedProviders(loginMethods: readonly string[] | undefined): string[] {
  return (loginMethods ?? []).filter((m) => m !== 'password');
}
