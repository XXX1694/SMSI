import { readFileSync } from 'node:fs';
import path from 'node:path';
import { describe, expect, it } from 'vitest';
// @ts-expect-error plain .mjs without types; shared with `npm run i18n:literals`
import { scanSource, scanTree } from '../scripts/i18n-literals.mjs';

interface Hit {
  file: string;
  line: number;
  kind: string;
  text: string;
}
const root = path.resolve(__dirname, '..');
const allow = JSON.parse(readFileSync(path.join(root, 'scripts/i18n-literals.allow.json'), 'utf8')) as {
  files: Record<string, string>;
  texts: Record<string, Record<string, string>>;
};

describe('hard-coded user-visible English', () => {
  it('finds JSX text, user-visible attributes and UI sentences', () => {
    const hits = scanSource(
      `export const A = () => (<div title="Close" aria-label={open ? 'Close menu' : 'Open menu'}>Hello <b>{'World'}</b><img alt="" /><input placeholder={\`Search\`} /><Field label="Name" hint={'Pick one'} /></div>);
       const m = { label: 'Draft', note: 'x' };
       toast.success('Saved.');
       const msg = 'Could not load the post.';`,
      'a.tsx',
    ) as Hit[];
    const kinds = hits.map((h) => `${h.kind}:${h.text}`);
    expect(kinds).toEqual(
      expect.arrayContaining([
        'attr:title:Close',
        'attr:aria-label:Close menu',
        'attr:aria-label:Open menu',
        'jsx-text:Hello',
        'jsx-expr:World',
        'attr:placeholder:Search',
        'attr:label:Name',
        'attr:hint:Pick one',
        'string:Draft',
        'string:Saved.',
        'string:Could not load the post.',
      ]),
    );
  });

  it('ignores identifiers, class names, punctuation and catalog calls', () => {
    const hits = scanSource(
      `import x from 'react';
       export const A = () => (<div className="flex items-center" aria-label={t('nav.label')}>{count} · {t('a.b')} <span>/</span> 12</div>);
       const scope = 'posts:read'; const h = { 'Content-Type': 'application/json' };
       type K = 'Draft' | 'Scheduled'; if (status === 'Draft') {}`,
      'b.tsx',
    ) as Hit[];
    expect(hits).toEqual([]);
  });

  it('leaves no English in components, pages or lib code outside the allow-list', () => {
    const hits = scanTree({ root, dirs: ['src/components', 'src/app', 'src/lib', 'src/hooks.ts'], allow }) as Hit[];
    const report = hits.map((h) => `${h.file}:${h.line} ${h.kind} "${h.text}"`);
    expect(report, 'Move these strings into messages/en.json (useTranslations), or add a reason to scripts/i18n-literals.allow.json').toEqual([]);
  });

  it('keeps the allow-list honest: every entry still points at a file', () => {
    for (const f of [...Object.keys(allow.files), ...Object.keys(allow.texts)]) {
      expect(() => readFileSync(path.join(root, f), 'utf8'), f).not.toThrow();
    }
  });
});
