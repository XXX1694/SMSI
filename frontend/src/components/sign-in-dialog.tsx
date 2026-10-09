'use client';
import { ReauthDialog } from '@/components/reauth-dialog';
import { useToast } from '@/components/toast';
import { useTranslations } from '@/i18n/use-translations';
import { ApiError, api } from '@/lib/api';
import { navigateTo } from '@/lib/navigate';

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
      if (!authorize_url) throw new ApiError(502, 'PROVIDER_ERROR', '');
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
      conflictText={connecting ? undefined : t('lastMethod')}
      onConfirm={confirm}
    />
  );
}
