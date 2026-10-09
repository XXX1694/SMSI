'use client';
import { usePathname, useRouter, useSearchParams } from 'next/navigation';
import { useEffect, useState } from 'react';
import type en from '../../messages/en.json';
import { Notice } from '@/components/states';
import { useTranslations } from '@/i18n/use-translations';
import { signInProviderKey } from '@/lib/sign-in-providers';

/**
 * The codes the API puts in `/settings?error=<code>` after a failed connect. The list is a subset of the login page's:
 * anything else gets the generic sentence, never the code.
 */
const CODES = [
  'oauth_cancelled',
  'oauth_state_invalid',
  'oauth_provider_error',
  'identity_in_use',
  'account_unavailable',
] as const satisfies readonly (keyof typeof en.settings.signIn.linkError)[];
type Code = (typeof CODES)[number];
const isCode = (c: string): c is Code => (CODES as readonly string[]).includes(c);

interface Failure {
  code: string;
  provider: string | null;
}

/**
 * Reads `?error=&provider=` once, removes them from the address and shows what went wrong. A successful connect carries
 * no parameter (the API redirects to plain `/settings`), and the list below is read fresh on arrival.
 */
export function LinkResultNotice() {
  const t = useTranslations('settings.signIn');
  const params = useSearchParams();
  const router = useRouter();
  const pathname = usePathname();
  const [failure, setFailure] = useState<Failure | null>(null);
  const code = params.get('error');
  const provider = params.get('provider');
  useEffect(() => {
    if (!code) return;
    setFailure({ code, provider });
    router.replace(pathname);
  }, [code, provider, router, pathname]);
  if (!failure) return null;
  // The provider is only ever a key of the select: an unknown or missing one reads as "other", and is never echoed.
  const text = isCode(failure.code) ? t(`linkError.${failure.code}`, { provider: signInProviderKey(failure.provider) }) : t('linkError.unknown');
  return <Notice tone="danger">{text}</Notice>;
}
