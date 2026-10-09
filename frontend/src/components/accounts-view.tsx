'use client';
import { usePathname, useRouter, useSearchParams } from 'next/navigation';
import Link from 'next/link';
import { useCallback, useEffect, useState } from 'react';
import { CapabilityBadges } from '@/components/capability-badges';
import { ConfirmDialog } from '@/components/confirm-dialog';
import { ErrorState, LoadingRows, Notice } from '@/components/states';
import { AccountStatusBadge } from '@/components/status-badge';
import { TelegramConnect } from '@/components/telegram-connect';
import { TokenConnectDialog } from '@/components/token-connect-dialog';
import { useToast } from '@/components/toast';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { api } from '@/lib/api';
import { DEMO } from '@/lib/demo/config';
import { describeErrorCode } from '@/lib/errors';
import { useProviderName } from '@/i18n/use-provider-name';
import type { Provider, SocialAccount } from '@/lib/types';
import { useAsync, useErrorText } from '@/hooks';
import { nodes } from '@/i18n/rich';
import type { AppT } from '@/i18n/translate';
import { useTranslations } from '@/i18n/use-translations';
import { useFormat } from '@/i18n/use-format';

function unavailableReason(p: Provider, t: AppT): string {
  // Each unavailable network states its own reason in capabilities.notes (Medium, X and Hashnode are not about approval).
  if (p.capabilities.notes) return p.capabilities.notes;
  if (!p.unsupported && !p.configured) return t('accounts.notConfigured');
  return t('accounts.notAvailableYet');
}

function AccountItem({ account, provider, onDisconnect, onConnected }: { account: SocialAccount; provider: Provider; onDisconnect: (a: SocialAccount) => void; onConnected: () => void }) {
  const t = useTranslations('accounts');
  const fmt = useFormat();
  const name = account.display_name || account.username;
  return (
    <li className="flex flex-wrap items-center justify-between gap-2 py-2">
      <div className="min-w-0">
        <p className="truncate text-sm font-medium">{account.display_name || account.username}</p>
        <p className="text-xs text-muted-foreground">
          {t('meta', { hasName: String(Boolean(account.username)), username: account.username ?? '', when: fmt.dateTime(account.connected_at) })}
        </p>
      </div>
      <div className="flex items-center gap-2">
        <AccountStatusBadge status={account.status} />
        {account.status === 'expired' ? <ConnectAction provider={provider} hasAccounts={false} label={t('reconnect')} onConnected={onConnected} /> : null}
        <Button variant="ghost" size="sm" onClick={() => onDisconnect(account)} aria-label={t('disconnectLabel', { account: name })}>
          {t('disconnect')}
        </Button>
      </div>
    </li>
  );
}

function DemoNote({ provider }: { provider: Provider }) {
  const t = useTranslations('accounts');
  if (provider.id === 'telegram') return null;
  const text = t(isTokenProvider(provider) ? 'demoToken' : 'demoOauth', { network: provider.name });
  return <p className="text-xs text-muted-foreground">{text}</p>;
}

function isTokenProvider(p: Provider): boolean {
  return p.capabilities.connectMethod === 'token' && p.capabilities.connectFields.length > 0;
}

/** The Connect button: a form for token providers, a redirect for OAuth ones, an instant sample account in the demo. Telegram has its own panel. */
function ConnectAction({ provider, hasAccounts, label: forced, onConnected }: { provider: Provider; hasAccounts: boolean; label?: string; onConnected: () => void }) {
  const t = useTranslations('accounts');
  const errorText = useErrorText();
  const toast = useToast();
  const [connecting, setConnecting] = useState(false);
  const [tokenOpen, setTokenOpen] = useState(false);
  const label = forced ?? (hasAccounts ? t('connectAnother') : t('connect'));

  async function connectDemo() {
    setConnecting(true);
    try {
      const account = await api.social.connectDemo(provider.id);
      toast.success(t('demoConnected', { name: account.display_name }));
      onConnected();
    } catch (e) {
      toast.error(errorText(e));
    } finally {
      setConnecting(false);
    }
  }

  if (isTokenProvider(provider)) {
    return (
      <>
        <Button variant="secondary" size="sm" onClick={() => setTokenOpen(true)} aria-haspopup="dialog">
          {label}
        </Button>
        <TokenConnectDialog
          provider={provider}
          open={tokenOpen}
          onOpenChange={setTokenOpen}
          onConnected={(account) => {
            setTokenOpen(false);
            toast.success(t('connected', { name: account.display_name || account.username || provider.name }));
            onConnected();
          }}
        />
      </>
    );
  }
  if (provider.id === 'telegram') return null;
  if (DEMO) {
    return (
      <Button variant="secondary" size="sm" onClick={() => void connectDemo()} disabled={connecting}>
        {label}
      </Button>
    );
  }
  return (
    <Button asChild variant="secondary" size="sm">
      <a href={api.social.connectUrl(provider.id)}>{label}</a>
    </Button>
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
  const t = useTranslations();
  const caps = provider.capabilities;
  if (!provider.available) {
    return (
      <li className="flex flex-wrap items-center gap-x-3 gap-y-1 py-3">
        <h2 className="text-sm font-medium">{provider.name}</h2>
        <Badge tone="warning">{t('accounts.notAvailable')}</Badge>
        <span className="text-sm text-muted-foreground">{unavailableReason(provider, t)}</span>
      </li>
    );
  }
  return (
    <li className="space-y-3 py-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <h2 className="text-sm font-semibold">{provider.name}</h2>
        </div>
        <ConnectAction provider={provider} hasAccounts={accounts.length > 0} onConnected={onConnected} />
      </div>
      <CapabilityBadges caps={caps} />
      {caps.notes ? <p className="text-xs text-muted-foreground">{caps.notes}</p> : null}
      {DEMO ? <DemoNote provider={provider} /> : null}
      {provider.id === 'telegram' ? <TelegramConnect onConnected={onConnected} /> : null}
      {accounts.length > 0 ? (
        <ul className="divide-y rounded-md border px-3" aria-label={t('accounts.listLabel', { network: provider.name })}>
          {accounts.map((a) => (
            <AccountItem key={a.id} account={a} provider={provider} onDisconnect={onDisconnect} onConnected={onConnected} />
          ))}
        </ul>
      ) : null}
    </li>
  );
}

interface ConnectResult {
  tone: 'info' | 'danger';
  text: string;
}

/** The message for `?connected=` / `?error=`, shown once; the parameters are then removed from the address. */
function useConnectResult(): ConnectResult | null {
  const t = useTranslations();
  const providerName = useProviderName();
  const params = useSearchParams();
  const router = useRouter();
  const pathname = usePathname();
  const [result, setResult] = useState<ConnectResult | null>(null);
  const connected = params.get('connected');
  const failed = params.get('error');
  const provider = params.get('provider');
  useEffect(() => {
    if (connected) setResult({ tone: 'info', text: t('accounts.resultConnected', { network: providerName(connected) }) });
    else if (failed) {
      const reason = describeErrorCode(failed, t);
      setResult({ tone: 'danger', text: provider ? t('accounts.resultFailed', { network: providerName(provider), reason }) : t('accounts.resultFailedGeneric', { reason }) });
    } else return;
    router.replace(pathname);
  }, [connected, failed, provider, router, pathname, t, providerName]);
  return result;
}

export function AccountsView() {
  const t = useTranslations('accounts');
  const providerName = useProviderName();
  const result = useConnectResult();
  const toast = useToast();
  const load = useCallback(async () => {
    const [providers, accounts] = await Promise.all([api.social.providers(), api.social.accounts()]);
    return { providers, accounts };
  }, []);
  const { data, error, loading, reload } = useAsync(load);
  const [target, setTarget] = useState<SocialAccount | null>(null);
  const targetName = target?.display_name || target?.username;
  const network = target ? providerName(target.provider) : '';
  const disconnectBody = targetName ? t('disconnectBody', { account: targetName, network }) : t('disconnectBodyUnnamed', { network });

  if (loading && !data) return <LoadingRows rows={4} />;
  if (error || !data) return <ErrorState error={error} onRetry={reload} />;

  // Available providers first, unavailable ones after.
  const providers = [...data.providers].sort((a, b) => Number(b.available) - Number(a.available));
  const known = new Set(providers.map((p) => p.id));
  const orphans = data.accounts.filter((a) => !known.has(a.provider));

  return (
    <div className="space-y-4">
      {result ? <Notice tone={result.tone}>{result.text}</Notice> : null}
      {data.accounts.length === 0 ? (
        <Notice tone="info">
          {nodes(
            t.rich('noneYet', {
              link: (c) => (
                <Link href="/compose" className="underline underline-offset-4">
                  {c}
                </Link>
              ),
            }),
          )}
        </Notice>
      ) : null}
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
      {orphans.length > 0 ? <p className="text-xs text-muted-foreground">{t('orphans', { count: orphans.length })}</p> : null}
      <ConfirmDialog
        open={target !== null}
        onOpenChange={(o) => !o && setTarget(null)}
        title={t('disconnectTitle')}
        description={disconnectBody}
        confirmLabel={t('disconnect')}
        destructive
        onConfirm={async () => {
          if (!target) return;
          await api.social.disconnect(target.id);
          toast.success(t('disconnected'));
          reload();
        }}
      />
    </div>
  );
}
