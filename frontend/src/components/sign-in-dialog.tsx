'use client';
import { ReauthDialog, ReauthFailure } from '@/components/reauth-dialog';
import { useToast } from '@/components/toast';
import { useTranslations } from '@/i18n/use-translations';
import { api } from '@/lib/api';
import { navigateTo } from '@/lib/navigate';
import { signInProviderKey } from '@/lib/sign-in-providers';

export interface Target {
  action: 'connect' | 'disconnect';
  id: string;
  name: string;
}

/** The confirm step of connecting or disconnecting one provider; connecting ends in a navigation to the provider. */
export function SignInDialog({ target, hasPassword, onClose, onDisconnected }: { target: Target; hasPassword: boolean; onClose: () => void; onDisconnected: () => void }) {
  const t = useTranslations('settings.signIn');
  const toast = useToast();
  const connecting = target.action === 'connect';

  async function confirm(password: string | undefined) {
    if (connecting) {
      const { authorize_url } = await api.auth.linkIdentity(target.id, password);
      // The API answered without a usable address: say the provider failed, not something about a social network.
      if (!authorize_url) throw new ReauthFailure(t('linkError.oauth_provider_error', { provider: signInProviderKey(target.id) }));
      navigateTo(authorize_url);
      return;
    }
    await api.auth.unlinkIdentity(target.id, password);
    toast.success(t('disconnected', { provider: target.name }));
    onDisconnected();
  }

  return (
    <ReauthDialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={t(connecting ? 'connectTitle' : 'disconnectTitle', { provider: target.name })}
      description={t(connecting ? 'connectBody' : 'disconnectBody', { provider: target.name })}
      confirmLabel={connecting ? t('connectConfirm', { provider: target.name }) : t('disconnectConfirm')}
      destructive={!connecting}
      needsPassword={hasPassword}
      statusText={connecting ? { 409: t('connectConflict'), 404: t('connectUnavailable') } : { 409: t('lastMethod'), 404: t('disconnectGone') }}
      onConfirm={confirm}
    />
  );
}
