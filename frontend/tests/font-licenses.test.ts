import { readFileSync } from 'node:fs';
import path from 'node:path';
import { describe, expect, it } from 'vitest';
import pkg from '../package.json';

// The OFL asks for the licence to travel with the fonts. The app ships them in public/licenses/fonts.txt (D-024); this
// keeps that file in step with the font packages the app depends on.
describe('font licences', () => {
  const notice = readFileSync(path.resolve(__dirname, '../public/licenses/fonts.txt'), 'utf8');
  const fonts = Object.keys(pkg.dependencies).filter((d) => d.startsWith('@fontsource'));

  it('covers every bundled font package', () => {
    expect(fonts.length).toBeGreaterThan(0);
    for (const name of fonts) {
      const licence = readFileSync(path.resolve(__dirname, '../node_modules', name, 'LICENSE'), 'utf8').trim();
      expect(notice, name).toContain(name);
      expect(notice, name).toContain(licence);
    }
  });
});
