import type {
  Capabilities,
  ConnectField,
  CreatedApiKey,
  CreatedMcpConnection,
  McpConfigSnippets,
  McpConnection,
  Me,
  ApiKey,
  Page,
  Provider,
  SocialAccount,
  TelegramLink,
  TelegramLinkState,
  TelegramLinkStatus,
} from './types';

type Rec = Record<string, unknown>;

function isRec(v: unknown): v is Rec {
  return typeof v === 'object' && v !== null && !Array.isArray(v);
}

/** Read a field accepting snake_case, camelCase or PascalCase spellings. */
function pick(rec: Rec, name: string): unknown {
  const snake = name.replace(/[A-Z]/g, (c) => `_${c.toLowerCase()}`);
  const pascal = name.charAt(0).toUpperCase() + name.slice(1);
  for (const k of [name, snake, pascal]) if (k in rec) return rec[k];
  return undefined;
}

export function normalizeMe(raw: unknown): Me {
  const r = isRec(raw) ? raw : {};
  const user = isRec(r.user) ? r.user : {};
  return {
    id: str(r.id),
    email: str(r.email),
    display_name: str(r.display_name),
    csrf_token: str(r.csrf_token),
    scopes: Array.isArray(r.scopes) ? r.scopes.filter((s): s is string => typeof s === 'string') : undefined,
    // Absent fields mean an older server or the demo, which never restrict anything.
    email_verified: user.email_verified !== false,
    verification_enforced: r.verification_enforced === true,
    mail_delivery: r.mail_delivery === 'log' ? 'log' : 'smtp',
    deletion_grace_days: typeof r.deletion_grace_days === 'number' && r.deletion_grace_days > 0 ? r.deletion_grace_days : 7,
    deletion_scheduled_at: typeof user.deletion_scheduled_at === 'string' ? user.deletion_scheduled_at : null,
  };
}

const bool = (v: unknown): boolean => v === true;
const num = (v: unknown): number => (typeof v === 'number' ? v : 0);
const str = (v: unknown): string => (typeof v === 'string' ? v : '');

export function normalizeCapabilities(raw: unknown): Capabilities {
  const r = isRec(raw) ? raw : {};
  return {
    canPublishText: bool(pick(r, 'canPublishText')),
    canPublishImage: bool(pick(r, 'canPublishImage')),
    canPublishVideo: bool(pick(r, 'canPublishVideo')),
    canSchedule: bool(pick(r, 'canSchedule')),
    canDelete: bool(pick(r, 'canDelete')),
    canAnalytics: bool(pick(r, 'canAnalytics')),
    maxTextLength: num(pick(r, 'maxTextLength')),
    maxMediaCount: num(pick(r, 'maxMediaCount')),
    requiresApproval: bool(pick(r, 'requiresApproval')),
    notes: str(pick(r, 'notes')),
    connectMethod: str(pick(r, 'connectMethod')),
    connectFields: normalizeConnectFields(pick(r, 'connectFields')),
  };
}

/** The string-valued entries of an object, e.g. the per-field messages of a validation error. */
export function stringRecord(raw: unknown): Record<string, string> {
  if (!isRec(raw)) return {};
  return Object.fromEntries(Object.entries(raw).filter((e): e is [string, string] => typeof e[1] === 'string'));
}

/** Fields without a name are dropped; an unknown kind is treated as plain text, a `secret` kind always as a secret. */
export function normalizeConnectFields(raw: unknown): ConnectField[] {
  if (!Array.isArray(raw)) return [];
  return raw.flatMap((item): ConnectField[] => {
    const f = isRec(item) ? item : {};
    const name = str(f.name);
    if (!name) return [];
    const kind = f.kind === 'secret' || f.kind === 'url' ? f.kind : 'text';
    return [
      {
        name,
        label: str(f.label) || name,
        help: str(f.help),
        placeholder: str(f.placeholder),
        kind,
        required: f.required === true,
        secret: f.secret === true || kind === 'secret',
      },
    ];
  });
}

export function normalizeProvider(raw: unknown): Provider {
  const r = isRec(raw) ? raw : {};
  const capabilities = normalizeCapabilities(pick(r, 'capabilities') ?? r);
  const id = str(pick(r, 'id')) || str(pick(r, 'provider')) || str(pick(r, 'name'));
  const status = str(pick(r, 'status'));
  const configured = pick(r, 'configured') !== false;
  const unsupported = status === 'unsupported' || pick(r, 'unsupported') === true;
  const canDoAnything =
    capabilities.canPublishText || capabilities.canPublishImage || capabilities.canPublishVideo;
  return {
    id,
    name: str(pick(r, 'displayName')) || str(pick(r, 'label')) || providerLabel(id),
    configured,
    unsupported,
    available: configured && !unsupported && canDoAnything,
    capabilities,
  };
}

const LABELS: Record<string, string> = {
  linkedin: 'LinkedIn',
  telegram: 'Telegram',
  mock: 'Test network',
  instagram: 'Instagram',
  facebook: 'Facebook',
  tiktok: 'TikTok',
  youtube: 'YouTube',
  x: 'X',
  threads: 'Threads',
  pinterest: 'Pinterest',
  discord: 'Discord',
  mastodon: 'Mastodon',
  bluesky: 'Bluesky',
};

/** Display name of a network id. An empty id gives an empty string; the caller supplies the "Unknown" text from the catalog. */
export function providerLabel(id: string): string {
  return LABELS[id] ?? (id ? id.charAt(0).toUpperCase() + id.slice(1) : '');
}

export function unwrapList<T>(raw: unknown): T[] {
  if (Array.isArray(raw)) return raw as T[];
  if (isRec(raw) && Array.isArray(raw.items)) return raw.items as T[];
  return [];
}

export function normalizePage<T>(raw: unknown): Page<T> {
  const next = isRec(raw) && typeof raw.next_cursor === 'string' ? raw.next_cursor : null;
  return { items: unwrapList<T>(raw), next_cursor: next };
}

/** Find the one-time raw key (`sk_…`) anywhere at the top level of a creation response. */
function findRawKey(raw: Rec): string {
  for (const k of ['raw_key', 'api_key', 'key', 'secret', 'token']) {
    const v = raw[k];
    if (typeof v === 'string' && v.startsWith('sk_')) return v;
  }
  for (const v of Object.values(raw)) if (typeof v === 'string' && v.startsWith('sk_')) return v;
  return '';
}

export function normalizeCreatedApiKey(raw: unknown): CreatedApiKey {
  const r = isRec(raw) ? raw : {};
  const meta = ['key', 'api_key', 'item'].map((k) => r[k]).find(isRec) ?? r;
  return { key: meta as unknown as ApiKey, rawKey: findRawKey(r) };
}

/** The only npm package a generated config may run. Pinned exactly: never `npx -y <name>` without a version. */
export const MCP_REMOTE_PACKAGE = 'mcp-remote@0.14.3';

/**
 * `stdio` is the Claude Desktop bridge: Desktop spawns `mcp-remote`, which talks HTTP to our server.
 * There is no Steerpost npm package (do not invent one: whoever registered the name would receive the keys).
 */
export function buildMcpConfig(rawKey: string, mcpUrl: string): McpConfigSnippets {
  const http = {
    mcpServers: {
      steerpost: { type: 'http', url: mcpUrl, headers: { Authorization: `Bearer ${rawKey}` } },
    },
  };
  const stdio = {
    mcpServers: {
      steerpost: {
        command: 'npx',
        args: ['-y', MCP_REMOTE_PACKAGE, mcpUrl, '--header', 'Authorization:${SOCIALOS_AUTH_HEADER}'],
        env: { SOCIALOS_AUTH_HEADER: `Bearer ${rawKey}` },
      },
    },
  };
  return { http: JSON.stringify(http, null, 2), stdio: JSON.stringify(stdio, null, 2) };
}

export function normalizeCreatedMcp(raw: unknown, mcpUrl: string): CreatedMcpConnection {
  const r = isRec(raw) ? raw : {};
  const meta = ['connection', 'mcp_connection', 'item'].map((k) => r[k]).find(isRec) ?? r;
  const rawKey = findRawKey(r);
  const cfg = isRec(r.config) ? r.config : null;
  const fromServer = (v: unknown): string | null =>
    typeof v === 'string' ? v : isRec(v) ? JSON.stringify(v, null, 2) : null;
  const built = buildMcpConfig(rawKey, mcpUrl);
  return {
    connection: meta as unknown as McpConnection,
    rawKey,
    config: {
      http: (cfg && fromServer(cfg.http)) ?? built.http,
      stdio: (cfg && fromServer(cfg.stdio)) ?? built.stdio,
    },
  };
}

export function normalizeTelegramLink(raw: unknown): TelegramLink {
  const r = isRec(raw) ? raw : {};
  return {
    id: str(r.id),
    code: str(r.code),
    expires_at: str(r.expires_at),
    bot_username: str(r.bot_username).replace(/^@/, ''),
    instructions: str(r.instructions),
  };
}

const LINK_STATUSES: readonly TelegramLinkStatus[] = ['pending', 'connected', 'expired'];

/** Unknown statuses are treated as pending: the next poll decides, nothing is ever wrongly "connected". */
export function normalizeTelegramLinkState(raw: unknown): TelegramLinkState {
  const r = isRec(raw) ? raw : {};
  const status = LINK_STATUSES.find((s) => s === r.status) ?? 'pending';
  return { status, account: isRec(r.account) ? (r.account as unknown as SocialAccount) : null };
}
