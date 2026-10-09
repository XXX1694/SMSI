'use client';
import { useState } from 'react';
import { api } from '@/lib/api';
import { describeConnectFailure, fieldPayload, isSecretField, requiredErrors } from '@/lib/token-connect';
import type { Provider, SocialAccount } from '@/lib/types';

/**
 * State of a "Connect with a token" form. Values live here only while the form is mounted (the dialog
 * unmounts it on close), and secret values are dropped as soon as they have been sent.
 */
export function useTokenConnect(provider: Provider, onConnected: (account: SocialAccount) => void, idPrefix: string) {
  const fields = provider.capabilities.connectFields;
  const [values, setValues] = useState<Record<string, string>>({});
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [formError, setFormError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  function set(name: string, value: string) {
    setValues((v) => ({ ...v, [name]: value }));
    setErrors((e) => (e[name] ? { ...e, [name]: '' } : e));
  }

  function dropSecrets() {
    setValues((v) => Object.fromEntries(Object.entries(v).filter(([name]) => !fields.some((f) => f.name === name && isSecretField(f)))));
  }

  async function submit() {
    setFormError(null);
    const missing = requiredErrors(fields, values);
    setErrors(missing);
    const first = fields.find((f) => missing[f.name]);
    if (first) {
      document.getElementById(`${idPrefix}-${first.name}`)?.focus();
      return;
    }
    setBusy(true);
    try {
      const account = await api.social.connectWithToken(provider.id, fieldPayload(fields, values));
      setValues({});
      onConnected(account);
    } catch (e) {
      const failure = describeConnectFailure(e, fields, provider.name);
      dropSecrets();
      setErrors(failure.fields);
      setFormError(failure.message);
      setBusy(false);
    }
  }

  return { values, errors, formError, busy, set, submit };
}
