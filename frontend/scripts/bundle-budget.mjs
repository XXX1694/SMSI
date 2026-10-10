#!/usr/bin/env node
/**
 * `npm run budget` (after `npm run build`): first-load JS per route, gzip level 6, against bundle-budget.json.
 * Issue #180: Next's table leaves the layout chunks out, which is where the regression hid.
 */
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { compareBudgets, filesContaining, formatTable, gzipBytes, routeScripts } from './bundle-budget-lib.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..');
const nextDir = join(root, '.next');
const readJson = (file) => JSON.parse(readFileSync(file, 'utf8'));
const read = (file) => readFileSync(join(nextDir, file));

// Strings that survive minification (component display names / a shell.json key). If one stops being emitted the
// guard would pass for the wrong reason, so the check also demands that the app layouts still carry them.
const RADIX_DIALOG = 'DialogContentModal';
const SHELL_KEY = 'skipToContent';
const AUTH_ROUTES = ['/login', '/register', '/verify-email'];

function main() {
  let manifests;
  let budgets;
  try {
    manifests = {
      appManifest: readJson(join(nextDir, 'app-build-manifest.json')),
      buildManifest: readJson(join(nextDir, 'build-manifest.json')),
    };
    budgets = readJson(join(root, 'bundle-budget.json'));
  } catch (error) {
    console.error(`bundle-budget: cannot read the build output or bundle-budget.json (${error.message}). Run \`npm run build\` first.`);
    return 2;
  }

  const scripts = {};
  const measured = {};
  for (const route of Object.keys(budgets)) {
    scripts[route] = routeScripts(route, manifests);
    measured[route] = scripts[route].reduce((sum, file) => sum + gzipBytes(read(file)), 0);
  }
  const rows = compareBudgets(measured, budgets);
  console.log(formatTable(rows));

  const problems = [];
  for (const row of rows.filter((r) => r.over)) {
    problems.push(`${row.route} is ${row.kb.toFixed(1)} kB gzip, budget ${row.budgetKb.toFixed(1)} kB.`);
  }

  const rootLayouts = manifests.appManifest.pages['/layout'].filter((f) => /\/app\/layout-[^/]*\.js$/.test(f));
  const rootLayoutText = Object.fromEntries(rootLayouts.map((f) => [f, read(f).toString('utf8')]));
  if (rootLayouts.length === 0) problems.push('guard: no app/layout-*.js chunk found in the root layout entry.');
  for (const file of filesContaining(rootLayoutText, SHELL_KEY)) {
    problems.push(`guard: "${SHELL_KEY}" (shell.json) is in the root layout chunk ${file}; shell text belongs in the (app) layout.`);
  }

  const authText = {};
  for (const route of AUTH_ROUTES.filter((r) => r in budgets)) {
    for (const file of scripts[route]) authText[file] ??= read(file).toString('utf8');
  }
  for (const file of filesContaining(authText, RADIX_DIALOG)) {
    problems.push(`guard: Radix Dialog ("${RADIX_DIALOG}") is in ${file}, loaded by an auth route; load dialogs lazily or only in (app).`);
  }
  const appChunks = Object.keys(manifests.appManifest.pages).filter((k) => k.startsWith('/(app)/')).flatMap((k) => manifests.appManifest.pages[k]);
  const appText = Object.fromEntries([...new Set(appChunks)].filter((f) => f.endsWith('.js')).map((f) => [f, read(f).toString('utf8')]));
  if (filesContaining(appText, RADIX_DIALOG).length === 0) {
    problems.push(`guard: "${RADIX_DIALOG}" no longer appears in any (app) chunk, so the Radix Dialog check cannot detect anything. Update RADIX_DIALOG in scripts/bundle-budget.mjs.`);
  }

  if (problems.length === 0) {
    console.log('\nbundle-budget: all routes within budget, guards clean.');
    return 0;
  }
  console.error(`\nbundle-budget: FAILED\n${problems.map((p) => `  - ${p}`).join('\n')}`);
  console.error(
    [
      '',
      'Find what grew: compare the table above with the same command on main (git worktree add ... origin/main).',
      'Raising a budget is a deliberate choice, not a fix: edit frontend/bundle-budget.json in the same PR and give the',
      'reason in the PR description and CHANGELOG. Budgets are a ratchet (AGENTS.md section 4): lower them when a route shrinks.',
    ].join('\n'),
  );
  return 1;
}

process.exit(main());
