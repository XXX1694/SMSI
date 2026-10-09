import { copyFileSync, mkdirSync, readFileSync } from 'node:fs';
import { basename, join } from 'node:path';

/**
 * Self-hosted fonts for the site (D-024), taken from the same @fontsource-variable packages the app bundles.
 *
 * A package's CSS has one @font-face per unicode-range slice (7 for Onest, about 120 for Noto Sans JP). `fontFaces` keeps
 * only the slices that cover characters the pages use, copies their files and the package's OFL licence to `outDir`, and
 * returns the CSS (urls relative to a stylesheet in `outDir`). The browser still applies unicode-range, so a slice that is
 * listed but not on screen is never downloaded.
 */

/** Parses "U+0400-045F,U+2116" into [[0x400, 0x45f], [0x2116, 0x2116]]. */
export function parseRanges(value) {
  return value.split(',').map((part) => {
    const [lo, hi] = part.trim().replace(/^U\+/i, '').split('-');
    const a = parseInt(lo, 16);
    return [a, hi ? parseInt(hi, 16) : a];
  });
}

/** The code points of `text`, without control characters (tabs and newlines would match no slice anyway). */
export function codePoints(text) {
  const set = new Set();
  for (const ch of text) {
    const p = ch.codePointAt(0);
    if (p >= 0x20) set.add(p);
  }
  return set;
}

function covers(ranges, points) {
  for (const p of points) if (ranges.some(([a, b]) => p >= a && p <= b)) return true;
  return false;
}

/**
 * @param {object} o
 * @param {string} o.pkgDir   the package folder, e.g. node_modules/@fontsource-variable/onest
 * @param {Set<number>} o.points  the code points the pages use
 * @param {string} o.outDir   where the font files and the licence go (dist/assets/fonts); the CSS must live there too
 * @param {'swap' | 'optional'} [o.display]  'optional' for script faces: a face that arrives late is skipped for that page
 *   view instead of re-flowing the text (the system face for the script stays), so it never causes a layout shift
 * @returns {string} the CSS
 */
export function fontFaces({ pkgDir, points, outDir, display = 'swap' }) {
  const source = readFileSync(join(pkgDir, 'wght.css'), 'utf8');
  const faces = source.match(/@font-face\s*\{[^}]*\}/g) ?? [];
  if (faces.length === 0) throw new Error(`${pkgDir}/wght.css has no @font-face rules`);
  mkdirSync(outDir, { recursive: true });
  // The OFL asks for the licence to travel with the font files.
  copyFileSync(join(pkgDir, 'LICENSE'), join(outDir, `${basename(pkgDir)}-OFL.txt`));
  const kept = [];
  for (const face of faces) {
    const range = face.match(/unicode-range:\s*([^;]+);/);
    const url = face.match(/url\(\.?\/?(files\/[^)]+\.woff2)\)/);
    if (!url) throw new Error(`unexpected @font-face in ${pkgDir}: ${face.slice(0, 80)}`);
    if (range && !covers(parseRanges(range[1]), points)) continue;
    const file = basename(url[1]);
    copyFileSync(join(pkgDir, url[1]), join(outDir, file));
    kept.push(face.replace(url[0], `url(${file})`).replace(/font-display:\s*\w+/, `font-display: ${display}`));
  }
  return `${kept.join('\n')}\n`;
}

/** The visible text of an HTML document: markup and inline scripts dropped, attributes (alt, aria-label, title) kept. */
export function pageText(html) {
  return html
    .replace(/<script\b[^>]*>[\s\S]*?<\/script[^>]*>/gi, ' ')
    .replace(/<style\b[^>]*>[\s\S]*?<\/style[^>]*>/gi, ' ')
    .replace(/<!--[\s\S]*?-->/g, ' ');
}
