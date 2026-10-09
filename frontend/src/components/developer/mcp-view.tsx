'use client';
import { useCallback, useState, type ReactNode } from 'react';
import { ConfirmDialog } from '@/components/confirm-dialog';
import { EmptyState, ErrorState, InlineError, LoadingRows, Notice } from '@/components/states';
import { Section } from '@/components/ui/card';
import { useToast } from '@/components/toast';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Field, Input } from '@/components/ui/input';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { api } from '@/lib/api';
import { joinList } from '@/lib/format';
import { MCP_PERMISSIONS, mcpDefaultSelection, permissionDescription, permissionLabel, permissionsToScopes } from '@/lib/scopes';
import { formatRelative } from '@/lib/time';
import type { CreatedMcpConnection, McpConnection } from '@/lib/types';
import { useAsync, useErrorText } from '@/hooks';
import { nodes } from '@/i18n/rich';
import { CodeBlock, CopyButton } from './copy-block';
import { useTranslations } from '@/i18n/use-translations';

const MCP_DOCS = 'https://github.com/XXX1694/steerpost/blob/main/mcp/README.md';
const code = (c: ReactNode) => <code>{c}</code>;

function CreatedPanel({ created, onDone }: { created: CreatedMcpConnection; onDone: () => void }) {
  const t = useTranslations('developer.mcp');
  const tc = useTranslations('common');
  return (
    <div className="space-y-4 rounded-lg border border-accent/40 p-4" data-testid="mcp-created">
      <div>
        <p className="text-sm font-semibold">{t('createdTitle', { name: created.connection.name })}</p>
        <p className="text-sm text-muted-foreground">{t('createdBody')}</p>
      </div>
      <Tabs defaultValue="http">
        <TabsList aria-label={t('clientLabel')}>
          <TabsTrigger value="http">{t('tabHttp')}</TabsTrigger>
          <TabsTrigger value="stdio">{t('tabStdio')}</TabsTrigger>
        </TabsList>
        <TabsContent value="http">
          <p className="mb-2 text-sm text-muted-foreground">
            {nodes(t.rich('httpIntro', { envKey: '${env:STEERPOST_API_KEY}', code }))}
          </p>
          <CodeBlock title={t('httpConfig')} code={created.config.http} />
        </TabsContent>
        <TabsContent value="stdio">
          <div className="mb-2 space-y-2 text-sm text-muted-foreground">
            <p>
              {nodes(
                t.rich('connectorIntro', {
                  header: 'Authorization: Bearer <key>',
                  b: (c) => <strong>{c}</strong>,
                  code,
                  link: (c) => (
                    <a className="text-accent underline-offset-4 hover:underline" href={MCP_DOCS} target="_blank" rel="noreferrer">
                      {c}
                    </a>
                  ),
                }),
              )}
            </p>
            <p>{nodes(t.rich('bridgeIntro', { b: (c) => <strong>{c}</strong>, code }))}</p>
          </div>
          <CodeBlock title={t('bridgeConfig')} code={created.config.stdio} />
        </TabsContent>
      </Tabs>
      <div className="flex items-center gap-2">
        <CopyButton text={created.rawKey} label={t('copyKeyOnly')} />
        <Button onClick={onDone}>{tc('haveSaved')}</Button>
      </div>
    </div>
  );
}

function CreateForm({ onCreated }: { onCreated: (c: CreatedMcpConnection) => void }) {
  const t = useTranslations();
  const errorText = useErrorText();
  const [name, setName] = useState('');
  const [perms, setPerms] = useState<string[]>(mcpDefaultSelection());
  const [ack, setAck] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const risky = MCP_PERMISSIONS.some((p) => perms.includes(p.id) && p.risk === 'dangerous');

  async function submit() {
    if (!name.trim()) return setError(t('developer.mcp.nameRequired'));
    if (perms.length === 0) return setError(t('developer.mcp.permsRequired'));
    if (risky && !ack) return setError(t('developer.mcp.ackRequired'));
    setBusy(true);
    setError(null);
    try {
      const c = await api.developer.createMcpConnection({ name: name.trim(), scopes: permissionsToScopes(perms) });
      setName('');
      setPerms(mcpDefaultSelection());
      setAck(false);
      onCreated(c);
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        void submit();
      }}
      className="max-w-xl space-y-5"
    >
      <Field label={t('developer.mcp.nameLabel')} htmlFor="mcp-name" hint={t('developer.mcp.nameHint')}>
        <Input id="mcp-name" value={name} onChange={(e) => setName(e.target.value)} maxLength={60} placeholder={t('developer.mcp.namePlaceholder')} />
      </Field>
      <fieldset className="space-y-3">
        <legend className="mb-1 text-sm font-medium">{t('developer.mcp.permissions')}</legend>
        {MCP_PERMISSIONS.map((p) => {
          const id = `mcp-perm-${p.id}`;
          return (
            <div key={p.id} className="flex items-start gap-2.5">
              <Checkbox id={id} checked={perms.includes(p.id)} onCheckedChange={(c) => setPerms((cur) => (c === true ? [...cur, p.id] : cur.filter((x) => x !== p.id)))} />
              <label htmlFor={id} className="text-sm">
                <span className="font-medium">{permissionLabel(p.id, t)}</span>{' '}
                {p.risk === 'dangerous' ? <Badge tone="danger">{t('developer.scopes.risk.dangerous')}</Badge> : p.risk === 'medium' ? <Badge tone="warning">{t('developer.scopes.risk.medium')}</Badge> : null}
                <span className="block text-xs text-muted-foreground">{permissionDescription(p.id, t)}</span>
              </label>
            </div>
          );
        })}
      </fieldset>
      {risky ? (
        <>
          <Notice>{t('developer.mcp.approvalNotice')}</Notice>
          <div className="flex items-start gap-2.5">
            <Checkbox id="mcp-ack" checked={ack} onCheckedChange={(c) => setAck(c === true)} />
            <label htmlFor="mcp-ack" className="text-sm">{t('developer.mcp.ackLabel')}</label>
          </div>
        </>
      ) : null}
      {error ? <InlineError>{error}</InlineError> : null}
      <Button type="submit" disabled={busy}>{busy ? t('common.creating') : t('developer.mcp.create')}</Button>
    </form>
  );
}

function ConnectionList({ items, onRevoke }: { items: McpConnection[]; onRevoke: (c: McpConnection) => void }) {
  const t = useTranslations();
  if (items.length === 0) return <EmptyState title={t('developer.mcp.emptyTitle')}>{t('developer.mcp.emptyBody')}</EmptyState>;
  return (
    <ul className="divide-y rounded-lg border">
      {items.map((c) => (
        <li key={c.id} className="flex flex-wrap items-center justify-between gap-3 px-4 py-3">
          <div className="min-w-0">
            <p className="text-sm font-medium">
              {c.name} {c.revoked_at ? <Badge>{t('common.status.account.revoked')}</Badge> : null}
            </p>
            <p className="text-xs text-muted-foreground">
              {t('developer.mcp.lastSeen', { hasClient: String(Boolean(c.client_name)), client: c.client_name ?? '', when: formatRelative(c.last_seen_at, t) })}
            </p>
            <p className="mt-1 text-xs text-muted-foreground">{joinList(c.scopes, t)}</p>
          </div>
          {c.revoked_at ? null : (
            <Button variant="ghost" size="sm" onClick={() => onRevoke(c)} aria-label={t('developer.apiKeys.revokeLabel', { name: c.name })}>
              {t('developer.apiKeys.revoke')}
            </Button>
          )}
        </li>
      ))}
    </ul>
  );
}

export function McpView() {
  const t = useTranslations();
  const load = useCallback(() => api.developer.mcpConnections(), []);
  const { data, error, loading, reload } = useAsync(load);
  const toast = useToast();
  const [created, setCreated] = useState<CreatedMcpConnection | null>(null);
  const [revoking, setRevoking] = useState<McpConnection | null>(null);

  return (
    <div className="space-y-10">
      <Section title={t('developer.mcp.connectSection')}>
        {created ? (
          <CreatedPanel created={created} onDone={() => setCreated(null)} />
        ) : (
          <CreateForm
            onCreated={(c) => {
              setCreated(c);
              reload();
            }}
          />
        )}
      </Section>
      <Section title={t('developer.mcp.listSection')}>
        {loading && !data ? <LoadingRows rows={2} /> : error || !data ? <ErrorState error={error} onRetry={reload} /> : <ConnectionList items={data} onRevoke={setRevoking} />}
      </Section>
      <ConfirmDialog
        open={revoking !== null}
        onOpenChange={(o) => !o && setRevoking(null)}
        title={t('developer.mcp.revokeTitle')}
        description={revoking ? t('developer.mcp.revokeBody', { name: revoking.name }) : ''}
        confirmLabel={t('developer.apiKeys.revoke')}
        destructive
        onConfirm={async () => {
          if (!revoking) return;
          await api.developer.revokeMcpConnection(revoking.id);
          toast.success(t('developer.mcp.revoked'));
          reload();
        }}
      />
    </div>
  );
}
