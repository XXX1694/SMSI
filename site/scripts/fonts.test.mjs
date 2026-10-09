import assert from 'node:assert/strict';
import { existsSync, mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { codePoints, fontFaces, pageText, parseRanges } from './fonts.mjs';

const here = dirname(fileURLToPath(import.meta.url));
const pkg = (name) => join(here, '../node_modules/@fontsource-variable', name);

test('pageText drops scripts and styles, including end tags with spaces or attributes', () => {
  const html = 'a<script type="x">日本</script >b<SCRIPT>中</script foo>c<style media="x">ع</style >d<!-- Қ -->e';
  assert.equal(pageText(html).replace(/\s+/g, ''), 'abcde');
});

test('codePoints skips control characters', () => {
  assert.deepEqual([...codePoints('a\n\tӘ')], [0x61, 0x4d8]);
});

test('parseRanges reads single points and spans', () => {
  assert.deepEqual(parseRanges('U+0400-045F, U+2116'), [[0x400, 0x45f], [0x2116, 0x2116]]);
});

test('fontFaces keeps only the slices the text needs and ships the licence', () => {
  const out = mkdtempSync(join(tmpdir(), 'fonts-'));
  try {
    const css = fontFaces({ pkgDir: pkg('onest'), points: codePoints('Hello Әә'), outDir: out, display: 'optional' });
    assert.match(css, /onest-latin-wght-normal\.woff2/);
    assert.match(css, /onest-cyrillic-ext-wght-normal\.woff2/);
    assert.doesNotMatch(css, /vietnamese|math/);
    assert.doesNotMatch(css, /font-display: swap/);
    assert.ok(existsSync(join(out, 'onest-latin-wght-normal.woff2')));
    assert.match(readFileSync(join(out, 'onest-OFL.txt'), 'utf8'), /SIL Open Font License/);
  } finally {
    rmSync(out, { recursive: true, force: true });
  }
});
