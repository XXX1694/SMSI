'use client';
import { useCallback, useState } from 'react';
import { ConfirmDialog } from '@/components/confirm-dialog';
import { EmptyState, ErrorState, InlineError, LoadingRows, Notice, Section } from '@/components/states';
import { useToast } from '@/components/toast';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Field, Input } from '@/components/ui/input';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { api } from '@/lib/api';
import { MCP_PERMISSIONS, mcpDefaultSelection, permissionsToScopes } from '@/lib/scopes';
import { formatRelative } from '@/lib/time';
import type { CreatedMcpConnection, McpConnection } from '@/lib/types';
import { errorMessage, useAsync } from '@/hooks';
import { CodeBlock, CopyButton } from './copy-block';

function CreatedPanel({ created, onDone }: { created: CreatedMcpConnection; onDone: () => void }) {
  return (
    <div className="space-y-4 rounded-lg border border-accent/40 p-4" data-testid="mcp-created">
      <div>
        <p className="text-sm font-semibold">Connection “{created.connection.name}” created</p>
        <p className="text-sm text-muted-foreground">This config contains the API key and is shown only once. Paste it into your MCP client now.</p>
      </div>
      <Tabs defaultValue="http">
        <TabsList aria-label="Transport">
          <TabsTrigger value="http">HTTP</TabsTrigger>
          <TabsTrigger value="stdio">stdio</TabsTrigger>
        </TabsList>
        <TabsContent value="http">
          <CodeBlock title="HTTP config" code={created.config.http} />
        </TabsContent>
        <TabsContent value="stdio">
          <CodeBlock title="stdio config" code={created.config.stdio} />
        </TabsContent>
      </Tabs>
      <div className="flex items-center gap-2">
        <CopyButton text={created.rawKey} label="Copy API key only" />
        <Button onClick={onDone}>I have saved it</Button>
      </div>
    </div>
  );
}

function CreateForm({ onCreated }: { onCreated: (c: CreatedMcpConnection) => void }) {
  const [name, setName] = useState('');
  const [perms, setPerms] = useState<string[]>(mcpDefaultSelection());
  const [ack, setAck] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const risky = MCP_PERMISSIONS.some((p) => perms.includes(p.id) && p.risk === 'dangerous');

  async function submit() {
    if (!name.trim()) return setError('Give the connection a name, e.g. “Claude Desktop”.');
    if (perms.length === 0) return setError('Select at least one permission.');
    if (risky && !ack) return setError('Confirm that you understand the risk of dangerous permissions.');
    setBusy(true);
    setError(null);
    try {
      const c = await api.developer.createMcpConnection({ name: name.trim(), scopes: permissionsToScopes(perms) });
      setName('');
      setPerms(mcpDefaultSelection());
      setAck(false);
      onCreated(c);
    } catch (e) {
      setError(errorMessage(e));
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
      <Field label="Connection name" htmlFor="mcp-name" hint="Shown in the list of connected agents.">
        <Input id="mcp-name" value={name} onChange={(e) => setName(e.target.value)} maxLength={60} placeholder="Claude Desktop" />
      </Field>
      <fieldset className="space-y-3">
        <legend className="mb-1 text-sm font-medium">Permissions</legend>
        {MCP_PERMISSIONS.map((p) => {
          const id = `mcp-perm-${p.id}`;
          return (
            <div key={p.id} className="flex items-start gap-2.5">
              <Checkbox id={id} checked={perms.includes(p.id)} onCheckedChange={(c) => setPerms((cur) => (c === true ? [...cur, p.id] : cur.filter((x) => x !== p.id)))} />
              <label htmlFor={id} className="text-sm">
                <span className="font-medium">{p.label}</span>{' '}
                {p.risk === 'dangerous' ? <Badge tone="danger">Dangerous</Badge> : p.risk === 'medium' ? <Badge tone="warning">Medium</Badge> : null}
                <span className="block text-xs text-muted-foreground">{p.description}</span>
              </label>
            </div>
          );
        })}
      </fieldset>
      {risky ? (
        <>
          <Notice>Dangerous tools still require the agent to pass an explicit confirm flag, but they can act on live accounts.</Notice>
          <div className="flex items-start gap-2.5">
            <Checkbox id="mcp-ack" checked={ack} onCheckedChange={(c) => setAck(c === true)} />
            <label htmlFor="mcp-ack" className="text-sm">I understand and want to grant these permissions.</label>
          </div>
        </>
      ) : null}
      {error ? <InlineError>{error}</InlineError> : null}
      <Button type="submit" disabled={busy}>{busy ? 'Creating…' : 'Create connection'}</Button>
    </form>
  );
}

function ConnectionList({ items, onRevoke }: { items: McpConnection[]; onRevoke: (c: McpConnection) => void }) {
  if (items.length === 0) return <EmptyState title="No agents connected">Create a connection above, then paste the config into your MCP client.</EmptyState>;
  return (
    <ul className="divide-y rounded-lg border">
      {items.map((c) => (
        <li key={c.id} className="flex flex-wrap items-center justify-between gap-3 px-4 py-3">
          <div className="min-w-0">
            <p className="text-sm font-medium">
              {c.name} {c.revoked_at ? <Badge>Revoked</Badge> : null}
            </p>
            <p className="text-xs text-muted-foreground">
              {c.client_name ? `${c.client_name} · ` : ''}last seen {formatRelative(c.last_seen_at)}
            </p>
            <p className="mt-1 text-xs text-muted-foreground">{c.scopes.join(', ')}</p>
          </div>
          {c.revoked_at ? null : (
            <Button variant="ghost" size="sm" onClick={() => onRevoke(c)} aria-label={`Revoke ${c.name}`}>
              Revoke
            </Button>
          )}
        </li>
      ))}
    </ul>
  );
}

export function McpView() {
  const load = useCallback(() => api.developer.mcpConnections(), []);
  const { data, error, loading, reload } = useAsync(load);
  const toast = useToast();
  const [created, setCreated] = useState<CreatedMcpConnection | null>(null);
  const [revoking, setRevoking] = useState<McpConnection | null>(null);

  return (
    <div className="space-y-10">
      <Section title="Connect an AI agent">
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
      <Section title="Connected agents">
        {loading && !data ? <LoadingRows rows={2} /> : error || !data ? <ErrorState error={error} onRetry={reload} /> : <ConnectionList items={data} onRevoke={setRevoking} />}
      </Section>
      <ConfirmDialog
        open={revoking !== null}
        onOpenChange={(o) => !o && setRevoking(null)}
        title="Revoke connection?"
        description={`“${revoking?.name ?? ''}” loses access immediately and its API key stops working.`}
        confirmLabel="Revoke"
        destructive
        onConfirm={async () => {
          if (!revoking) return;
          await api.developer.revokeMcpConnection(revoking.id);
          toast.success('Connection revoked');
          reload();
        }}
      />
    </div>
  );
}
