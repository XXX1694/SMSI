'use client';
import { KeyRound } from 'lucide-react';
import type { ReactNode } from 'react';
import { ProviderMark } from '@/components/provider-icons';
import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { useFormat } from '@/i18n/use-format';
import { useTranslations } from '@/i18n/use-translations';
import { DEMO } from '@/lib/demo/config';
import type { Identity } from '@/lib/types';

/** One way to enter the account: a mark, a name, lines of status and at most one action. */
export function SignInMethodRow({ providerId, name, status, action }: { providerId: string | null; name: string; status: ReactNode; action: ReactNode }) {
  return (
    <Card as="li" className="flex flex-wrap items-center gap-3">
      <span aria-hidden className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-muted text-muted-foreground">
        {providerId ? <ProviderMark id={providerId} className="h-4 w-4" /> : <KeyRound className="h-4 w-4" />}
      </span>
      <div className="min-w-0 flex-1 basis-48">
        <p className="text-sm font-medium">{name}</p>
        <div className="break-words text-xs text-muted-foreground">{status}</div>
      </div>
      {action}
    </Card>
  );
}

export interface ProviderEntry {
  id: string;
  name: string;
  /** The server no longer offers this provider, but the user is still connected to it: only Disconnect remains. */
  switchedOff: boolean;
}

/** A provider row: connected (email, date, Disconnect) or not (Connect). The demo has no provider to visit and says so. */
export function ProviderRow({ entry, identity, onConnect, onDisconnect }: { entry: ProviderEntry; identity: Identity | undefined; onConnect: () => void; onDisconnect: () => void }) {
  const t = useTranslations('settings.signIn');
  const fmt = useFormat();
  let status: ReactNode;
  let action: ReactNode = null;
  if (identity) {
    status = (
      <>
        <p>{t('connected', { hasEmail: String(Boolean(identity.email)), email: identity.email, when: fmt.dateTime(identity.linked_at) })}</p>
        {entry.switchedOff ? <p>{t('disabledNote', { provider: entry.name })}</p> : null}
      </>
    );
    action = (
      <Button variant="secondary" size="sm" aria-label={t('disconnectAria', { provider: entry.name })} onClick={onDisconnect}>
        {t('disconnect')}
      </Button>
    );
  } else if (DEMO) {
    status = <p>{t('demoNote')}</p>;
  } else {
    status = <p>{t('notConnected')}</p>;
    action = (
      <Button size="sm" aria-label={t('connectAria', { provider: entry.name })} onClick={onConnect}>
        {t('connect')}
      </Button>
    );
  }
  return <SignInMethodRow providerId={entry.id} name={entry.name} status={status} action={action} />;
}
