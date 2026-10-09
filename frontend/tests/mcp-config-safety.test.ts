import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { describe, expect, it } from 'vitest';
import { buildMcpConfig } from '@/lib/normalize';

const ROOT = resolve(__dirname, '../..');
// Every place that emits or documents client config. Deploy files legitimately name the published docker image.
const SCANNED = ['README.md', 'mcp/README.md', 'docs', 'site/src', 'frontend/src', 'backend/internal/application'];
const TEXT = /\.(md|html|ts|tsx|go|json|mjs)$/;
const EXACT = /^[@a-z0-9][\w./-]*@\d+\.\d+\.\d+$/;

function files(path: string): string[] {
  const full = join(ROOT, path);
  if (statSync(full).isFile()) return [full];
  return readdirSync(full, { withFileTypes: true }).flatMap((e) => {
    if (e.name === 'node_modules' || e.name === '.next') return [];
    const rel = join(path, e.name);
    return e.isDirectory() ? files(rel) : TEXT.test(e.name) ? [join(ROOT, rel)] : [];
  });
}

/** npx packages in text: the first non-flag token after `npx`, or after an args array that starts with "-y". */
function npxPackages(text: string): string[] {
  const out: string[] = [];
  for (const m of text.matchAll(/npx\s+(?:(?:-y|--yes)\s+)?([^\s"'`<-][^\s"'`<]*)/g)) out.push(m[1]!);
  for (const m of text.matchAll(/\[\s*['"]-y['"],\s*['"]([^'"]+)['"]/g)) out.push(m[1]!);
  return out;
}

describe('MCP client config safety (issue #90)', () => {
  const sources = SCANNED.flatMap(files).filter((f) => !/\.test\.|_test\.go$/.test(f));

  it('scans a non-trivial set of files', () => {
    expect(sources.length).toBeGreaterThan(20);
  });

  it('never references the unpublished socialos-mcp package', () => {
    const hits = sources.filter((f) => /socialos-mcp/.test(readFileSync(f, 'utf8')));
    expect(hits).toEqual([]);
  });

  it('pins every npx package to an exact version', () => {
    const bad = sources.flatMap((f) =>
      npxPackages(readFileSync(f, 'utf8'))
        .filter((p) => !EXACT.test(p))
        .map((p) => `${f.replace(ROOT, '')}: ${p}`),
    );
    expect(bad).toEqual([]);
  });

  it('generates a Claude Desktop config with a pinned package and the key only in env', () => {
    const { http, stdio } = buildMcpConfig('sk_live_abc', 'https://mcp.example.com/mcp');
    for (const cfg of [http, stdio]) expect(cfg).not.toContain('socialos-mcp');
    const srv = JSON.parse(stdio).mcpServers.steerpost as { command: string; args: string[]; env: Record<string, string> };
    expect(srv.command).toBe('npx');
    expect(srv.args[0]).toBe('-y');
    expect(srv.args[1]).toMatch(EXACT);
    expect(srv.args.join(' ')).not.toContain('sk_live_abc');
    expect(srv.env.SOCIALOS_AUTH_HEADER).toBe('Bearer sk_live_abc');
  });
});
