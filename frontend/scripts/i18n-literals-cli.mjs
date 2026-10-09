#!/usr/bin/env node
/** `npm run i18n:literals`: lists hard-coded user-visible English (same rules as tests/i18n-literals.test.ts). */
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { scanTree } from './i18n-literals.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..');
const allow = JSON.parse(readFileSync(join(root, 'scripts/i18n-literals.allow.json'), 'utf8'));
const hits = scanTree({ root, dirs: ['src/components', 'src/app', 'src/lib', 'src/hooks.ts'], allow });
for (const h of hits) console.error(`${h.file}:${h.line} ${h.kind} "${h.text}"`);
console.log(`i18n:literals ${hits.length} hard-coded strings`);
process.exit(hits.length ? 1 : 0);
