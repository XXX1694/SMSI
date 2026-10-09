'use client';
import { useCallback, useState } from 'react';
import { useAuth } from '@/components/auth-provider';
import { SetPasswordForm } from '@/components/set-password-form';
import { SignInDialog, type Target } from '@/components/sign-in-dialog';
import { ProviderRow, SignInMethodRow, type ProviderEntry } from '@/components/sign-in-method-row';
import { useSignInProviders } from '@/components/social-sign-in';
import { ErrorState, InlineError, LoadingRows } from '@/components/states';
import { Button } from '@/components/ui/button';
import { useAsync } from '@/hooks';
import { useProviderName } from '@/i18n/use-provider-name';
import { useTranslations } from '@/i18n/use-translations';
import { api } from '@/lib/api';
import type { Identity } from '@/lib/types';

/** Offered providers first, then providers the user is connected to that the server no longer offers (Disconnect only). */
function providerEntries(offered: { id: string; name: string }[], identities: Identity[], nameOf: (id: string) => string): ProviderEntry[] {
  const known = new Set(offered.map((p) => p.id));
  const switchedOff = identities.filter((i) => !known.has(i.provider)).map((i) => ({ id: i.provider, name: nameOf(i.provider), switchedOff: true }));
  return [...offered.map((p) => ({ ...p, switchedOff: false })), ...switchedOff];
}

/** Settings: how this account can be entered (D-023). */
export function SignInMethods() {
  const t = useTranslations('settings.signIn');
  const nameOf = useProviderName();
  const { refresh } = useAuth();
  const load = useCallback(() => api.auth.signInMethods(), []);
  const methods = useAsync(load);
  const enabled = useSignInProviders();
  const [target, setTarget] = useState<Target | null>(null);
  const [settingPassword, setSettingPassword] = useState(false);

  if (methods.loading || enabled.loading) return <LoadingRows rows={2} />;
  if (methods.error || !methods.data) return <ErrorState error={methods.error} title={t('loadFailed')} onRetry={methods.reload} />;
  const { identities, has_password: hasPassword } = methods.data;
  // A failed provider list is not "no providers": linked ones stay visible as plain connected rows, with a retry below.
  const entries = enabled.error ? identities.map((i) => ({ id: i.provider, name: nameOf(i.provider), switchedOff: false })) : providerEntries(enabled.providers, identities, nameOf);

  function changed() {
    methods.reload();
    void refresh();
  }

  return (
    <div className="space-y-3">
      <p className="text-sm text-muted-foreground">{t('intro')}</p>
      <ul className="space-y-2">
        <SignInMethodRow
          providerId={null}
          name={t('password')}
          status={<p>{hasPassword ? t('passwordOn') : t('passwordOff')}</p>}
          action={
            hasPassword || settingPassword ? null : (
              <Button size="sm" onClick={() => setSettingPassword(true)}>
                {t('setPasswordOpen')}
              </Button>
            )
          }
        />
        {entries.map((entry) => (
          <ProviderRow
            key={entry.id}
            entry={entry}
            identity={identities.find((i) => i.provider === entry.id)}
            onConnect={() => setTarget({ action: 'connect', id: entry.id, name: entry.name })}
            onDisconnect={() => setTarget({ action: 'disconnect', id: entry.id, name: entry.name })}
          />
        ))}
      </ul>
      {settingPassword && !hasPassword ? (
        <SetPasswordForm
          onDone={() => {
            setSettingPassword(false);
            changed();
          }}
          onCancel={() => setSettingPassword(false)}
        />
      ) : null}
      {enabled.error ? (
        <InlineError onRetry={enabled.reload}>{t('providersFailed')}</InlineError>
      ) : entries.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('noProviders')}</p>
      ) : null}
      {target ? <SignInDialog target={target} hasPassword={hasPassword} onClose={() => setTarget(null)} onDisconnected={changed} /> : null}
    </div>
  );
}
