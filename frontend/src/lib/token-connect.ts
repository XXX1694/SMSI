import { ApiError } from '@/lib/api';
import { errorMessage } from '@/hooks';
import type { AppT } from '@/i18n/translate';
import type { ConnectField } from '@/lib/types';

const DOCS_BASE = 'https://github.com/XXX1694/steerpost/blob/main/docs/integrations';

/** The how-to for a provider (docs/integrations/<provider>.md). */
export function integrationDocsUrl(provider: string): string {
  return `${DOCS_BASE}/${encodeURIComponent(provider)}.md`;
}

export function isSecretField(f: ConnectField): boolean {
  return f.secret || f.kind === 'secret';
}

/** Client-side check that mirrors the server: a required field must not be blank. Values are never echoed. */
export function requiredErrors(fields: ConnectField[], values: Record<string, string>, t: AppT): Record<string, string> {
  const out: Record<string, string> = {};
  for (const f of fields) if (f.required && !(values[f.name] ?? '').trim()) out[f.name] = t('accounts.tokenConnect.required', { field: f.label });
  return out;
}

/** The request body: trimmed, and without optional fields the user left empty. */
export function fieldPayload(fields: ConnectField[], values: Record<string, string>): Record<string, string> {
  const out: Record<string, string> = {};
  for (const f of fields) {
    const v = (values[f.name] ?? '').trim();
    if (v) out[f.name] = v;
  }
  return out;
}

export interface ConnectFailure {
  /** A sentence for the whole form, or null when only fields are wrong. */
  message: string | null;
  fields: Record<string, string>;
}

/** Server field messages (English) this form knows, mapped to catalog keys. */
const FIELD_TEXT = {
  required: 'accounts.tokenConnect.required',
  'must be an https URL': 'accounts.tokenConnect.httpsUrl',
  'too long': 'accounts.tokenConnect.tooLong',
} as const;
const isKnownField = (text: string): text is keyof typeof FIELD_TEXT => Object.hasOwn(FIELD_TEXT, text);

/**
 * Plain-English version of a failed token connect. Server messages are only used for the field they name;
 * nothing here ever contains a value the user typed.
 */
export function describeConnectFailure(e: unknown, form: ConnectField[], providerName: string, t: AppT): ConnectFailure {
  if (!(e instanceof ApiError)) return { message: errorMessage(e, t), fields: {} };
  const fields: Record<string, string> = {};
  const known = new Map(form.map((f) => [f.name, f]));
  let stray = false;
  for (const [name, text] of Object.entries(e.fields)) {
    const f = known.get(name);
    if (!f) stray = true;
    else fields[name] = t(isKnownField(text) ? FIELD_TEXT[text] : 'accounts.tokenConnect.invalid', { field: f.label });
  }
  if (Object.keys(fields).length > 0 && !stray) return { message: null, fields };
  if (stray) return { message: t('accounts.tokenConnect.outOfDate'), fields };
  return { message: describeStatus(e, providerName, t), fields };
}

function describeStatus(e: ApiError, providerName: string, t: AppT): string {
  if (e.status === 429 || e.code === 'RATE_LIMITED') return t('accounts.tokenConnect.rateLimited');
  if (e.code === 'INSUFFICIENT_SCOPE') return t('accounts.tokenConnect.noScope');
  if (e.status === 403 && e.code !== 'EMAIL_NOT_VERIFIED') return t('accounts.tokenConnect.forbidden');
  if (e.status === 400 && e.code === 'VALIDATION_ERROR') return t('accounts.tokenConnect.rejected', { network: providerName });
  if (e.status === 502 || e.code === 'PROVIDER_ERROR') return t('accounts.tokenConnect.unreachable', { network: providerName });
  return errorMessage(e, t);
}
