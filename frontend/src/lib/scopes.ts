import type { Scope } from './types';

export type ScopeRisk = 'safe' | 'medium' | 'dangerous';

export interface ScopeInfo {
  scope: Scope;
  label: string;
  description: string;
  risk: ScopeRisk;
}

export const SCOPES: readonly ScopeInfo[] = [
  { scope: 'social:read', label: 'Read accounts', description: 'List connected accounts and providers.', risk: 'safe' },
  { scope: 'posts:read', label: 'Read posts', description: 'List posts, statuses and attempts.', risk: 'safe' },
  { scope: 'analytics:read', label: 'Read analytics', description: 'Read analytics data.', risk: 'safe' },
  { scope: 'posts:write', label: 'Create and edit drafts', description: 'Create drafts, edit and cancel posts.', risk: 'safe' },
  { scope: 'media:write', label: 'Upload media', description: 'Upload files to the media library.', risk: 'safe' },
  { scope: 'posts:schedule', label: 'Schedule posts', description: 'Queue posts for later publication.', risk: 'medium' },
  { scope: 'posts:publish', label: 'Publish immediately', description: 'Publish to live social accounts right now.', risk: 'dangerous' },
  { scope: 'posts:delete', label: 'Delete posts', description: 'Permanently remove posts.', risk: 'dangerous' },
  { scope: 'social:disconnect', label: 'Disconnect accounts', description: 'Remove connected social accounts.', risk: 'dangerous' },
];

export const RISK_ORDER: readonly ScopeRisk[] = ['safe', 'medium', 'dangerous'];

export const RISK_LABEL: Record<ScopeRisk, string> = {
  safe: 'Safe',
  medium: 'Medium',
  dangerous: 'Dangerous',
};

export function groupScopes(): Record<ScopeRisk, ScopeInfo[]> {
  const out: Record<ScopeRisk, ScopeInfo[]> = { safe: [], medium: [], dangerous: [] };
  for (const s of SCOPES) out[s.risk].push(s);
  return out;
}

export function scopeRisk(scope: string): ScopeRisk | null {
  return SCOPES.find((s) => s.scope === scope)?.risk ?? null;
}

/** Default selection for API keys: all safe scopes, never dangerous ones. */
export function defaultScopes(): Scope[] {
  return SCOPES.filter((s) => s.risk === 'safe').map((s) => s.scope);
}

export function hasDangerous(scopes: readonly string[]): boolean {
  return scopes.some((s) => scopeRisk(s) === 'dangerous');
}

export interface McpPermission {
  id: string;
  label: string;
  description: string;
  scopes: Scope[];
  risk: ScopeRisk;
  defaultOn: boolean;
}

export const MCP_PERMISSIONS: readonly McpPermission[] = [
  { id: 'read', label: 'Read posts', description: 'Agents can list accounts, posts, statuses and analytics.', scopes: ['social:read', 'posts:read', 'analytics:read'], risk: 'safe', defaultOn: true },
  { id: 'draft', label: 'Create drafts', description: 'Agents can create and edit drafts and upload media.', scopes: ['posts:write', 'media:write'], risk: 'safe', defaultOn: true },
  { id: 'schedule', label: 'Schedule', description: 'Agents can queue posts for later publication.', scopes: ['posts:schedule'], risk: 'medium', defaultOn: false },
  { id: 'publish', label: 'Publish', description: 'Agents can publish to live accounts immediately.', scopes: ['posts:publish'], risk: 'dangerous', defaultOn: false },
  { id: 'delete', label: 'Delete', description: 'Agents can delete posts.', scopes: ['posts:delete'], risk: 'dangerous', defaultOn: false },
  { id: 'disconnect', label: 'Disconnect', description: 'Agents can disconnect social accounts.', scopes: ['social:disconnect'], risk: 'dangerous', defaultOn: false },
];

export function mcpDefaultSelection(): string[] {
  return MCP_PERMISSIONS.filter((p) => p.defaultOn).map((p) => p.id);
}

/** Expand selected permission ids into a de-duplicated scope list. */
export function permissionsToScopes(ids: readonly string[]): Scope[] {
  const set = new Set<Scope>();
  for (const p of MCP_PERMISSIONS) {
    if (ids.includes(p.id)) p.scopes.forEach((s) => set.add(s));
  }
  return [...set];
}
