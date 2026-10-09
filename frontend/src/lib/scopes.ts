import type { AppT } from '@/i18n/translate';
import type { Scope } from './types';

export type ScopeRisk = 'safe' | 'medium' | 'dangerous';

/** Names and descriptions live in the catalog: `scopes.<scope with ":" as "_">.label` (see `scopeLabel`). */
export interface ScopeInfo {
  scope: Scope;
  risk: ScopeRisk;
}

export const SCOPES: readonly ScopeInfo[] = [
  { scope: 'social:read', risk: 'safe' },
  { scope: 'posts:read', risk: 'safe' },
  { scope: 'analytics:read', risk: 'safe' },
  { scope: 'posts:write', risk: 'safe' },
  { scope: 'media:write', risk: 'safe' },
  { scope: 'posts:schedule', risk: 'medium' },
  { scope: 'posts:publish', risk: 'dangerous' },
  { scope: 'posts:delete', risk: 'dangerous' },
  { scope: 'social:disconnect', risk: 'dangerous' },
  { scope: 'social:connect', risk: 'dangerous' },
];

export const RISK_ORDER: readonly ScopeRisk[] = ['safe', 'medium', 'dangerous'];

export const riskLabel = (risk: ScopeRisk, t: AppT): string => t(`developer.scopes.risk.${risk}`);

type ScopeKey = 'social_read' | 'posts_read' | 'analytics_read' | 'posts_write' | 'media_write' | 'posts_schedule' | 'posts_publish' | 'posts_delete' | 'social_disconnect' | 'social_connect';
const scopeKey = (scope: Scope): ScopeKey => scope.replace(':', '_') as ScopeKey;
export const scopeLabel = (scope: Scope, t: AppT): string => t(`developer.scopes.${scopeKey(scope)}.label`);
export const scopeDescription = (scope: Scope, t: AppT): string => t(`developer.scopes.${scopeKey(scope)}.description`);

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
  id: 'read' | 'draft' | 'schedule' | 'publish' | 'delete' | 'disconnect';
  scopes: Scope[];
  risk: ScopeRisk;
  defaultOn: boolean;
}

export const MCP_PERMISSIONS: readonly McpPermission[] = [
  { id: 'read', scopes: ['social:read', 'posts:read', 'analytics:read'], risk: 'safe', defaultOn: true },
  { id: 'draft', scopes: ['posts:write', 'media:write'], risk: 'safe', defaultOn: true },
  { id: 'schedule', scopes: ['posts:schedule'], risk: 'medium', defaultOn: false },
  { id: 'publish', scopes: ['posts:publish'], risk: 'dangerous', defaultOn: false },
  { id: 'delete', scopes: ['posts:delete'], risk: 'dangerous', defaultOn: false },
  { id: 'disconnect', scopes: ['social:disconnect'], risk: 'dangerous', defaultOn: false },
];

export const permissionLabel = (id: McpPermission['id'], t: AppT): string => t(`developer.mcp.perms.${id}.label`);
export const permissionDescription = (id: McpPermission['id'], t: AppT): string => t(`developer.mcp.perms.${id}.description`);

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
