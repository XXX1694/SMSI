'use client';
import type en from '../../messages/en.json';
import { Notice } from '@/components/states';
import { useTranslations } from '@/i18n/use-translations';
import { signInProviderName } from '@/lib/sign-in-providers';

/**
 * The codes the API puts in `/login?error=<code>` after a provider round trip, plus `signup_expired`, which the
 * sign-up page adds itself when its ticket is gone. Anything else gets the generic sentence, never the code.
 */
const CODES = [
  'oauth_cancelled',
  'oauth_state_invalid',
  'oauth_provider_error',
  'email_unverified',
  'account_exists',
  'identity_in_use',
  'account_unavailable',
  'signup_expired',
] as const satisfies readonly (keyof typeof en.auth.loginError)[];
type Code = (typeof CODES)[number];
const isCode = (c: string): c is Code => (CODES as readonly string[]).includes(c);

export function LoginErrorNotice({ code, provider }: { code: string; provider: string | null }) {
  const t = useTranslations('auth');
  // The API does not always say which provider failed, so the sentence falls back to a neutral phrase.
  const name = signInProviderName(provider) ?? t('providerFallback');
  return (
    <div className="mt-4">
      <Notice tone="danger">{isCode(code) ? t(`loginError.${code}`, { provider: name }) : t('loginError.unknown')}</Notice>
    </div>
  );
}
