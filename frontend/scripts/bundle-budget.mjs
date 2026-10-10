#!/usr/bin/env node
/**
 * `npm run budget` (after `npm run build`): first-load JS per route, gzip level 6, against bundle-budget.json.
 * Issue #180: Next's table leaves the layout chunks out, which is where the regression hid.
 */
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { compareBudgets, formatTable, gzipBytes, markerProblems, routeScripts } from './bundle-budget-lib.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..');
const nextDir = join(root, '.next');
const readJson = (file) => JSON.parse(readFileSync(file, 'utf8'));
const read = (file) => readFileSync(join(nextDir, file));

// Strings that survive minification (a component display name, a shell.json key). markerProblems also fails when a
// marker is gone from every (app) chunk, so renaming one cannot make its guard pass silently.
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

  const readAll = (files) => Object.fromEntries([...new Set(files)].filter((f) => f.endsWith('.js')).map((f) => [f, read(f).toString('utf8')]));
  const pages = manifests.appManifest.pages;
  const appText = readAll(Object.keys(pages).filter((k) => k.startsWith('/(app)/')).flatMap((k) => pages[k]));
  const rootLayoutText = readAll(pages['/layout']);
  const authText = readAll(AUTH_ROUTES.filter((r) => r in budgets).flatMap((r) => scripts[r]));
  problems.push(
    ...markerProblems({
      marker: SHELL_KEY,
      label: 'shell.json text',
      forbidden: { ...rootLayoutText, ...authText },
      expected: appText,
      fix: 'shell text belongs in the (app) layout only.',
    }),
    ...markerProblems({
      marker: RADIX_DIALOG,
      label: 'Radix Dialog',
      forbidden: authText,
      expected: appText,
      fix: 'load dialogs lazily or only in (app).',
    }),
  );

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
