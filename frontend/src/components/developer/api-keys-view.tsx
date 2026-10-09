'use client';
import { useCallback, useState } from 'react';
import { ConfirmDialog } from '@/components/confirm-dialog';
import { usePrefs } from '@/components/prefs-provider';
import { EmptyState, ErrorState, InlineError, LoadingRows } from '@/components/states';
import { useToast } from '@/components/toast';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Table, Tbody, Td, Th, Thead, Tr } from '@/components/ui/table';
import { Dialog, DialogContent, DialogFooter } from '@/components/ui/dialog';
import { Field, Input, Select } from '@/components/ui/input';
import { api } from '@/lib/api';
import { defaultScopes, hasDangerous, scopeRisk } from '@/lib/scopes';
import { formatDateTime, formatRelative } from '@/lib/time';
import type { ApiKey, CreatedApiKey } from '@/lib/types';
import { errorMessage, useAsync } from '@/hooks';
import { CopyButton } from './copy-block';
import { ScopePicker } from './scope-picker';

function RawKeyDialog({ created, onClose }: { created: CreatedApiKey | null; onClose: () => void }) {
  return (
    <Dialog open={created !== null} onOpenChange={(o) => !o && onClose()}>
      <DialogContent title="Copy your API key" description="This is the only time the full key is shown. Store it somewhere safe.">
        <div className="space-y-3">
          <code data-testid="raw-key" className="block break-all rounded-md border bg-muted p-3 font-mono text-xs">
            {created?.rawKey}
          </code>
          <CopyButton text={created?.rawKey ?? ''} label="Copy key" />
        </div>
        <DialogFooter>
          <Button onClick={onClose}>I have saved it</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

const EXPIRY: { label: string; days: number | null }[] = [
  { label: 'Never', days: null },
  { label: '30 days', days: 30 },
  { label: '90 days', days: 90 },
  { label: '1 year', days: 365 },
];

function CreateKeyDialog({ open, onOpenChange, onCreated }: { open: boolean; onOpenChange: (o: boolean) => void; onCreated: (c: CreatedApiKey) => void }) {
  const [name, setName] = useState('');
  const [scopes, setScopes] = useState<string[]>(defaultScopes());
  const [expiry, setExpiry] = useState('90');
  const [ack, setAck] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const dangerous = hasDangerous(scopes);

  async function submit() {
    if (!name.trim()) return setError('Give the key a name.');
    if (scopes.length === 0) return setError('Select at least one scope.');
    if (dangerous && !ack) return setError('Confirm that you understand the risk of dangerous scopes.');
    setBusy(true);
    setError(null);
    try {
      const days = expiry ? Number(expiry) : null;
      const expires_at = days ? new Date(Date.now() + days * 86_400_000).toISOString() : undefined;
      const created = await api.developer.createApiKey({ name: name.trim(), scopes, expires_at });
      setName('');
      setScopes(defaultScopes());
      setAck(false);
      onOpenChange(false);
      onCreated(created);
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent title="Create API key" description="Keys act as you. Grant only what the integration needs.">
        <div className="space-y-5">
          <Field label="Name" htmlFor="key-name">
            <Input id="key-name" value={name} onChange={(e) => setName(e.target.value)} maxLength={60} placeholder="CI publisher" />
          </Field>
          <Field label="Expires" htmlFor="key-exp">
            <Select id="key-exp" value={expiry} onChange={(e) => setExpiry(e.target.value)}>
              {EXPIRY.map((o) => (
                <option key={o.label} value={o.days ?? ''}>{o.label}</option>
              ))}
            </Select>
          </Field>
          <ScopePicker value={scopes} onChange={setScopes} />
          {dangerous ? (
            <div className="flex items-start gap-2.5">
              <Checkbox id="key-ack" checked={ack} onCheckedChange={(c) => setAck(c === true)} />
              <label htmlFor="key-ack" className="text-sm">I understand this key can publish, delete or disconnect on my behalf.</label>
            </div>
          ) : null}
          {error ? <InlineError>{error}</InlineError> : null}
        </div>
        <DialogFooter>
          <Button variant="secondary" onClick={() => onOpenChange(false)}>Cancel</Button>
          <Button onClick={() => void submit()} disabled={busy}>{busy ? 'Creating…' : 'Create key'}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function KeyRow({ k, onRevoke }: { k: ApiKey; onRevoke: (k: ApiKey) => void }) {
  const { timezone } = usePrefs();
  const expired = k.expires_at !== null && new Date(k.expires_at).getTime() < Date.now();
  return (
    <Tr className="align-top">
      <Td label="Name" className="py-3">
        <div>
          <p className="font-medium">{k.name}</p>
          <code className="text-xs text-muted-foreground">{k.prefix}…</code>
        </div>
      </Td>
      <Td label="Scopes" className="py-3">
        <div className="flex max-w-xs flex-wrap gap-1">
          {k.scopes.map((s) => (
            <Badge key={s} tone={scopeRisk(s) === 'dangerous' ? 'danger' : scopeRisk(s) === 'medium' ? 'warning' : 'neutral'}>
              {s}
            </Badge>
          ))}
        </div>
      </Td>
      <Td label="Expires" className="whitespace-nowrap py-3 text-muted-foreground">{k.expires_at ? formatDateTime(k.expires_at, timezone) : 'Never'}</Td>
      <Td label="Last used" className="whitespace-nowrap py-3 text-muted-foreground">{formatRelative(k.last_used_at)}</Td>
      <Td align="right" className="py-3">
        {k.revoked_at ? <Badge>Revoked</Badge> : expired ? <Badge tone="warning">Expired</Badge> : (
          <Button variant="ghost" size="sm" onClick={() => onRevoke(k)} aria-label={`Revoke ${k.name}`}>Revoke</Button>
        )}
      </Td>
    </Tr>
  );
}

export function ApiKeysView() {
  const load = useCallback(() => api.developer.apiKeys(), []);
  const { data, error, loading, reload } = useAsync(load);
  const toast = useToast();
  const [createOpen, setCreateOpen] = useState(false);
  const [created, setCreated] = useState<CreatedApiKey | null>(null);
  const [revoking, setRevoking] = useState<ApiKey | null>(null);

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-3">
        <p className="text-sm text-muted-foreground">Use API keys for scripts and integrations. Send as <code className="text-xs">Authorization: Bearer sk_live_…</code></p>
        <Button onClick={() => setCreateOpen(true)}>Create key</Button>
      </div>
      {loading && !data ? (
        <LoadingRows rows={2} />
      ) : error || !data ? (
        <ErrorState error={error} onRetry={reload} />
      ) : data.length === 0 ? (
        <EmptyState title="No API keys">Create a key to call the REST API from your own tools.</EmptyState>
      ) : (
        <Table label="API keys">
          <Thead>
            <Tr>
              <Th>Name</Th>
              <Th>Scopes</Th>
              <Th>Expires</Th>
              <Th>Last used</Th>
              <Th><span className="sr-only">Actions</span></Th>
            </Tr>
          </Thead>
          <Tbody>
            {data.map((k) => (
              <KeyRow key={k.id} k={k} onRevoke={setRevoking} />
            ))}
          </Tbody>
        </Table>
      )}
      <CreateKeyDialog open={createOpen} onOpenChange={setCreateOpen} onCreated={(c) => { setCreated(c); reload(); }} />
      <RawKeyDialog created={created} onClose={() => setCreated(null)} />
      <ConfirmDialog
        open={revoking !== null}
        onOpenChange={(o) => !o && setRevoking(null)}
        title="Revoke API key?"
        description={`Anything using “${revoking?.name ?? 'this key'}” stops working immediately. This cannot be undone.`}
        confirmLabel="Revoke"
        destructive
        onConfirm={async () => {
          if (!revoking) return;
          await api.developer.revokeApiKey(revoking.id);
          toast.success('Key revoked');
          reload();
        }}
      />
    </div>
  );
}
