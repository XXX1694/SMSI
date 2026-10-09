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

  it('finds copy in any non-technical prop, including objects and arrays', () => {
    const hits = scanSource(
      `export const A = () => (<div><Empty empty="Nothing scheduled." /><List items={['Draft', 'Done']} /><Card copy={{ title: 'Heading', n: 3 }} /><Btn variant="primary" className="Flex" data-x="Hello there" aria-controls="Menu" onClick={() => go('Draft')} /></div>);`,
      'p.tsx',
    ) as Hit[];
    const kinds = hits.map((h) => `${h.kind}:${h.text}`);
    expect(kinds).toEqual(expect.arrayContaining(['attr:empty:Nothing scheduled.', 'attr:items:Draft', 'attr:items:Done', 'attr:copy:Heading']));
    expect(kinds.filter((k) => /variant|className|data-x|aria-controls/.test(k))).toEqual([]);
  });

  it('finds capitalised single-word labels in records, arrays, returns and variables', () => {
    const hits = scanSource(
      `const LABELS = { draft: 'Draft', ok: 'fine' };
       const ALL = ['Scheduled', 'x'];
       function f(s: string) { if (s === 'Draft') return 'Published'; return cond ? 'Failed' : 'failed'; }
       const one = 'Canceled';
       const list: string[] = []; list.push('Queued'); const m = new Map(); m.set('k', 'Sent');
       const x = a === 'Skipped' ? 1 : 2;`,
      'c.ts',
    ) as Hit[];
    expect(hits.map((h) => h.text).sort()).toEqual(['Canceled', 'Draft', 'Failed', 'Published', 'Queued', 'Scheduled', 'Sent'].sort());
  });

  it('checks the middle and end of template strings, not only the start', () => {
    const hits = scanSource(
      'const a = `${n} posts scheduled`; const b = `${n} of ${m}`; const c = `${base}/api/posts/${id}`; const d = `px-2 ${x} text-sm`; const e = `Hello ${name}`;',
      't.ts',
    ) as Hit[];
    expect(hits.map((h) => h.text)).toEqual(['posts scheduled', 'Hello']);
  });

  it('flags sentences passed to calls that used to be skipped (replace, set, push)', () => {
    const hits = scanSource(`s.replace('x', 'Could not load it.'); m.set('k', 'Could not save it.'); l.push('Nothing here.'); router.replace('/login');`, 'r.ts') as Hit[];
    expect(hits.map((h) => h.text)).toEqual(['Could not load it.', 'Could not save it.', 'Nothing here.']);
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
