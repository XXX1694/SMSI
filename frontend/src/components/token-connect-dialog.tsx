'use client';
import { ExternalLink } from 'lucide-react';
import { useId } from 'react';
import { InlineError } from '@/components/states';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogFooter } from '@/components/ui/dialog';
import { Field, Input } from '@/components/ui/input';
import { SecretInput } from '@/components/ui/secret-input';
import { useTokenConnect } from '@/components/use-token-connect';
import { integrationDocsUrl, isSecretField } from '@/lib/token-connect';
import type { AppT } from '@/i18n/translate';
import type { ConnectField, Provider, SocialAccount } from '@/lib/types';
import { useTranslations } from '@/i18n/use-translations';

function FormField({
  field,
  id,
  value,
  error,
  onChange,
}: {
  field: ConnectField;
  id: string;
  value: string;
  error: string;
  onChange: (value: string) => void;
}) {
  const optional = !field.required && !/optional/i.test(field.label);
  const props = {
    name: field.name,
    value,
    placeholder: field.placeholder || undefined,
    onChange: (e: React.ChangeEvent<HTMLInputElement>) => onChange(e.target.value),
  };
  return (
    <Field label={field.label} optional={optional} required={field.required} htmlFor={id} hint={field.help} error={error || null}>
      {isSecretField(field) ? (
        <SecretInput {...props} label={field.label} />
      ) : (
        <Input {...props} type={field.kind === 'url' ? 'url' : 'text'} autoComplete="off" autoCapitalize="off" spellCheck={false} />
      )}
    </Field>
  );
}

function TokenConnectForm({ provider, onConnected, onCancel }: { provider: Provider; onConnected: (a: SocialAccount) => void; onCancel: () => void }) {
  const t = useTranslations();
  const prefix = useId();
  const form = useTokenConnect(provider, onConnected, prefix);
  const { notes, connectFields } = provider.capabilities;
  return (
    <form
      noValidate
      autoComplete="off"
      onSubmit={(e) => {
        e.preventDefault();
        void form.submit();
      }}
      className="space-y-4"
    >
      {notes ? <p className="text-xs text-muted-foreground">{notes}</p> : null}
      <a
        href={integrationDocsUrl(provider.id)}
        target="_blank"
        rel="noopener noreferrer"
        className="inline-flex items-center gap-1 text-sm underline underline-offset-2"
      >
        {t('accounts.tokenConnect.howTo', { network: provider.name })}
        <ExternalLink className="h-3.5 w-3.5" aria-hidden />
        <span className="sr-only">{t('accounts.tokenConnect.newTab')}</span>
      </a>
      {connectFields.map((f) => (
        <FormField
          key={f.name}
          field={f}
          id={`${prefix}-${f.name}`}
          value={form.values[f.name] ?? ''}
          error={form.errors[f.name] ?? ''}
          onChange={(v) => form.set(f.name, v)}
        />
      ))}
      {form.formError ? <InlineError>{form.formError}</InlineError> : null}
      <DialogFooter>
        <Button type="button" variant="secondary" onClick={onCancel}>
          {t('common.cancel')}
        </Button>
        <Button type="submit" disabled={form.busy}>
          {form.busy ? t('accounts.tokenConnect.connecting') : t('accounts.tokenConnect.connectNetwork', { network: provider.name })}
        </Button>
      </DialogFooter>
    </form>
  );
}

/** "Connect with a token": a form built entirely from the provider's `connect_fields`. Closing it discards every value. */
/** The HTTPS promise is made only when this page is served over HTTPS; local installs use http://localhost. */
function storageNote(t: AppT): string {
  const https = typeof window !== 'undefined' && window.location.protocol === 'https:';
  return t(https ? 'accounts.tokenConnect.noteHttps' : 'accounts.tokenConnect.noteHttp');
}

export function TokenConnectDialog({
  provider,
  open,
  onOpenChange,
  onConnected,
}: {
  provider: Provider;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onConnected: (account: SocialAccount) => void;
}) {
  const t = useTranslations();
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent title={t('accounts.tokenConnect.connectNetwork', { network: provider.name })} description={storageNote(t)}>
        <TokenConnectForm provider={provider} onConnected={onConnected} onCancel={() => onOpenChange(false)} />
      </DialogContent>
    </Dialog>
  );
}
