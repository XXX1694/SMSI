import { describe, expect, it } from 'vitest';
import { defaultScopes, groupScopes, hasDangerous, MCP_PERMISSIONS, mcpDefaultSelection, permissionsToScopes, scopeRisk, SCOPES } from '@/lib/scopes';

describe('scope grouping', () => {
  it('groups every scope exactly once', () => {
    const g = groupScopes();
    expect(g.safe.length + g.medium.length + g.dangerous.length).toBe(SCOPES.length);
  });

  it('classifies publish, delete, connect and disconnect as dangerous and schedule as medium', () => {
    expect(groupScopes().dangerous.map((s) => s.scope).sort()).toEqual(['posts:delete', 'posts:publish', 'social:connect', 'social:disconnect']);
    expect(scopeRisk('posts:schedule')).toBe('medium');
    expect(scopeRisk('posts:read')).toBe('safe');
    expect(scopeRisk('unknown')).toBeNull();
  });

  it('never selects dangerous scopes by default', () => {
    expect(hasDangerous(defaultScopes())).toBe(false);
    expect(hasDangerous(['posts:read', 'posts:publish'])).toBe(true);
  });
});

describe('MCP permissions', () => {
  it('defaults to read + drafts only', () => {
    expect(mcpDefaultSelection()).toEqual(['read', 'draft']);
    expect(hasDangerous(permissionsToScopes(mcpDefaultSelection()))).toBe(false);
  });

  it('expands permissions to unique scopes', () => {
    const s = permissionsToScopes(['read', 'publish', 'read']);
    expect(s).toContain('posts:publish');
    expect(new Set(s).size).toBe(s.length);
  });

  it('maps every dangerous permission to dangerous scopes only', () => {
    for (const p of MCP_PERMISSIONS.filter((x) => x.risk === 'dangerous')) {
      expect(p.defaultOn).toBe(false);
      expect(p.scopes.every((s) => scopeRisk(s) === 'dangerous')).toBe(true);
    }
  });
});
