import { ApiError } from '@/lib/api';
import { errorMessage } from '@/hooks';
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
export function requiredErrors(fields: ConnectField[], values: Record<string, string>): Record<string, string> {
  const out: Record<string, string> = {};
  for (const f of fields) if (f.required && !(values[f.name] ?? '').trim()) out[f.name] = `${f.label} is required.`;
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

const FIELD_TEXT: Record<string, (label: string) => string> = {
  required: (l) => `${l} is required.`,
  'must be an https URL': (l) => `${l} must be an https address, for example https://example.com.`,
  'too long': (l) => `${l} is too long.`,
};

/**
 * Plain-English version of a failed token connect. Server messages are only used for the field they name;
 * nothing here ever contains a value the user typed.
 */
export function describeConnectFailure(e: unknown, form: ConnectField[], providerName: string): ConnectFailure {
  if (!(e instanceof ApiError)) return { message: errorMessage(e), fields: {} };
  const fields: Record<string, string> = {};
  const known = new Map(form.map((f) => [f.name, f]));
  let stray = false;
  for (const [name, text] of Object.entries(e.fields)) {
    const f = known.get(name);
    if (!f) stray = true;
    else fields[name] = (FIELD_TEXT[text] ?? ((l) => `${l} is not valid.`))(f.label);
  }
  if (Object.keys(fields).length > 0 && !stray) return { message: null, fields };
  if (stray) return { message: 'This form is out of date. Reload the page and try again.', fields };
  return { message: describeStatus(e, providerName), fields };
}

function describeStatus(e: ApiError, providerName: string): string {
  if (e.status === 429 || e.code === 'RATE_LIMITED') return 'Too many attempts. Wait a minute, then try again.';
  if (e.code === 'INSUFFICIENT_SCOPE') return 'This sign-in is not allowed to connect accounts. Use a key with the social:connect permission.';
  if (e.status === 403 && e.code !== 'EMAIL_NOT_VERIFIED') return 'You do not have permission to connect accounts.';
  if (e.status === 400 && e.code === 'VALIDATION_ERROR') {
    return `${providerName} did not accept these details. Check them with “How to connect ${providerName}” and try again. Nothing was saved.`;
  }
  if (e.status === 502 || e.code === 'PROVIDER_ERROR') {
    return `${providerName} could not be reached to check these details. Try again in a moment.`;
  }
  return errorMessage(e);
}
