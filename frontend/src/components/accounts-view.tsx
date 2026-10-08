'use client';
import { useSearchParams } from 'next/navigation';
import { useCallback, useState } from 'react';
import { CapabilityBadges } from '@/components/capability-badges';
import { ConfirmDialog } from '@/components/confirm-dialog';
import { usePrefs } from '@/components/prefs-provider';
import { ErrorState, LoadingRows, Notice } from '@/components/states';
import { AccountStatusBadge } from '@/components/status-badge';
import { TelegramConnect } from '@/components/telegram-connect';
import { useToast } from '@/components/toast';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { api } from '@/lib/api';
import { DEMO } from '@/lib/demo/config';
import { formatDateTime } from '@/lib/time';
import type { Provider, SocialAccount } from '@/lib/types';
import { errorMessage, useAsync } from '@/hooks';

function unavailableReason(p: Provider): string {
  if (p.unsupported || p.capabilities.requiresApproval) return 'Not supported yet. Requires platform approval.';
  if (!p.configured) return 'Not configured on this server.';
  return 'Not supported yet.';
}

function AccountItem({ account, onDisconnect }: { account: SocialAccount; onDisconnect: (a: SocialAccount) => void }) {
  const { timezone } = usePrefs();
  return (
    <li className="flex flex-wrap items-center justify-between gap-2 py-2">
      <div className="min-w-0">
        <p className="truncate text-sm font-medium">{account.display_name || account.username}</p>
        <p className="text-xs text-muted-foreground">
          {account.username ? `${account.username} · ` : ''}connected {formatDateTime(account.connected_at, timezone)}
        </p>
      </div>
      <div className="flex items-center gap-2">
        <AccountStatusBadge status={account.status} />
        <Button variant="ghost" size="sm" onClick={() => onDisconnect(account)} aria-label={`Disconnect ${account.display_name || account.username}`}>
          Disconnect
        </Button>
      </div>
    </li>
  );
}

function ProviderRow({
  provider,
  accounts,
  onDisconnect,
  onConnected,
}: {
  provider: Provider;
  accounts: SocialAccount[];
  onDisconnect: (a: SocialAccount) => void;
  onConnected: () => void;
}) {
  const caps = provider.capabilities;
  const toast = useToast();
  const [connecting, setConnecting] = useState(false);

  /** Demo only: there is no OAuth server to talk to, so the sample account appears immediately. */
  async function connectDemo() {
    setConnecting(true);
    try {
      const account = await api.social.connectDemo(provider.id);
      toast.success(`Connected ${account.display_name} (demo account, no real sign-in)`);
      onConnected();
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      setConnecting(false);
    }
  }

  if (!provider.available) {
    return (
      <li className="flex flex-wrap items-center gap-x-3 gap-y-1 py-3">
        <h2 className="text-sm font-medium">{provider.name}</h2>
        <Badge tone="warning">Not available</Badge>
        <span className="text-sm text-muted-foreground">{unavailableReason(provider)}</span>
      </li>
    );
  }
  return (
    <li className="space-y-3 py-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <h2 className="text-sm font-semibold">{provider.name}</h2>
          {provider.id === 'mock' ? <Badge tone="outline">Mock · for testing</Badge> : null}
        </div>
        {provider.id !== 'telegram' ? (
          DEMO ? (
            <Button variant="secondary" size="sm" onClick={() => void connectDemo()} disabled={connecting}>
              {accounts.length ? 'Connect another' : 'Connect'}
            </Button>
          ) : (
            <Button asChild variant="secondary" size="sm">
              <a href={api.social.connectUrl(provider.id)}>{accounts.length ? 'Connect another' : 'Connect'}</a>
            </Button>
          )
        ) : null}
      </div>
      <CapabilityBadges caps={caps} />
      {caps.notes ? <p className="text-xs text-muted-foreground">{caps.notes}</p> : null}
      {DEMO && provider.id !== 'telegram' ? (
        <p className="text-xs text-muted-foreground">Demo: the real {provider.name} sign-in is skipped; Connect adds a sample account.</p>
      ) : null}
      {caps.requiresApproval ? (
        <p className="text-xs text-muted-foreground">Some features (e.g. company pages) require platform approval.</p>
      ) : null}
      {provider.id === 'telegram' ? <TelegramConnect onConnected={onConnected} /> : null}
      {accounts.length > 0 ? (
        <ul className="divide-y rounded-md border px-3" aria-label={`${provider.name} accounts`}>
          {accounts.map((a) => (
            <AccountItem key={a.id} account={a} onDisconnect={onDisconnect} />
          ))}
        </ul>
      ) : null}
    </li>
  );
}

export function AccountsView() {
  const params = useSearchParams();
  const toast = useToast();
  const load = useCallback(async () => {
    const [providers, accounts] = await Promise.all([api.social.providers(), api.social.accounts()]);
    return { providers, accounts };
  }, []);
  const { data, error, loading, reload } = useAsync(load);
  const [target, setTarget] = useState<SocialAccount | null>(null);

  if (loading && !data) return <LoadingRows rows={4} />;
  if (error || !data) return <ErrorState error={error} onRetry={reload} />;

  const connected = params.get('connected');
  const failed = params.get('error');
  // Available providers first, unavailable ones after.
  const providers = [...data.providers].sort((a, b) => Number(b.available) - Number(a.available));
  const known = new Set(providers.map((p) => p.id));
  const orphans = data.accounts.filter((a) => !known.has(a.provider));

  return (
    <div className="space-y-4">
      {connected ? <Notice tone="info">Connected {connected} successfully.</Notice> : null}
      {failed ? <Notice tone="danger">Connection failed ({failed}). Please try again.</Notice> : null}
      <ul className="divide-y border-y">
        {providers.map((p) => (
          <ProviderRow
            key={p.id}
            provider={p}
            accounts={data.accounts.filter((a) => a.provider === p.id)}
            onDisconnect={setTarget}
            onConnected={reload}
          />
        ))}
      </ul>
      {orphans.length > 0 ? <p className="text-xs text-muted-foreground">{orphans.length} account(s) belong to providers no longer offered.</p> : null}
      <ConfirmDialog
        open={target !== null}
        onOpenChange={(o) => !o && setTarget(null)}
        title="Disconnect account?"
        description={`Scheduled posts targeting ${target?.display_name || target?.username || 'this account'} will no longer publish. You can reconnect later.`}
        confirmLabel="Disconnect"
        destructive
        onConfirm={async () => {
          if (!target) return;
          await api.social.disconnect(target.id);
          toast.success('Account disconnected');
          reload();
        }}
      />
    </div>
  );
}
