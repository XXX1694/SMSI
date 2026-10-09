#!/usr/bin/env node
/**
 * Builds the browser-only demo: a static export of the real app wired to the in-memory mock API.
 *
 *   npm run build:demo                                   # -> out/, for https://<user>.github.io/steerpost/demo/
 *   NEXT_PUBLIC_BASE_PATH= npm run build:demo            # -> out/, for the domain root (local `npx serve out`)
 *   NEXT_PUBLIC_BASE_PATH=/preview npm run build:demo    # any other sub-path
 *
 * It shares .next with `npm run build`, so do not run the two at the same time.
 */
import { spawnSync } from 'node:child_process';
import { existsSync, rmSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = join(dirname(fileURLToPath(import.meta.url)), '..');
const out = join(root, 'out');
rmSync(out, { recursive: true, force: true });

const res = spawnSync(process.execPath, [join(root, 'node_modules/next/dist/bin/next'), 'build'], {
  cwd: root,
  stdio: 'inherit',
  env: { ...process.env, NEXT_PUBLIC_DEMO: 'true', NEXT_TELEMETRY_DISABLED: '1' },
});
if (res.status !== 0) process.exit(res.status ?? 1);
if (!existsSync(join(out, 'index.html'))) {
  console.error('build:demo: expected a static export in out/ but found none');
  process.exit(1);
}
console.log(`build:demo: static demo written to ${out} (base path "${process.env.NEXT_PUBLIC_BASE_PATH ?? '/steerpost/demo'}")`);
