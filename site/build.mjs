#!/usr/bin/env node
/**
 * Builds the SocialOS website into site/dist:
 *   /                landing page
 *   /docs/...        documentation rendered from the repo's markdown (README.md, docs/GETTING-STARTED.md,
 *                    docs/integrations/README.md, docs/API.md, docs/ARCHITECTURE.md, mcp/README.md)
 *   /demo/           the browser-only demo (the static export from frontend/, `npm run build:demo`)
 *   /assets/...      css, js, font, screenshots
 *
 * Environment:
 *   SITE_BASE      URL path the site is served from (default /SMSI/, as on GitHub Pages project sites)
 *   SITE_URL       origin, used for canonical links and the sitemap (default https://xxx1694.github.io)
 *   SITE_REPO_URL  repository URL for "Source" links (default https://github.com/XXX1694/SMSI)
 *   DEMO_DIR       the demo export to copy to /demo (default ../frontend/out)
 * Flags:
 *   --no-demo      skip the demo (docs-only preview)
 */
import { cpSync, existsSync, mkdirSync, readFileSync, readdirSync, rmSync, statSync, writeFileSync } from 'node:fs';
import { dirname, join, posix, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import hljs from 'highlight.js';
import { Marked } from 'marked';

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(here, '..');
const dist = join(here, 'dist');
const src = join(here, 'src');

const normalizeBase = (b) => `/${b.replace(/^\/+|\/+$/g, '')}/`.replace(/^\/\/$/, '/');
const BASE = normalizeBase(process.env.SITE_BASE ?? '/SMSI/');
const SITE_URL = (process.env.SITE_URL ?? 'https://xxx1694.github.io').replace(/\/+$/, '');
const REPO_URL = (process.env.SITE_REPO_URL ?? 'https://github.com/XXX1694/SMSI').replace(/\/+$/, '');
const DEMO_DIR = resolve(here, process.env.DEMO_DIR ?? '../frontend/out');
const WITH_DEMO = !process.argv.includes('--no-demo');

const read = (p) => readFileSync(p, 'utf8');
const esc = (s) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
const fail = (msg) => {
  console.error(`site build: ${msg}`);
  process.exit(1);
};

// ------------------------------------------------------------------ markdown

/** Where each markdown file now lives on the site, for rewriting relative links. */
const PAGE_OF = {
  'README.md': 'docs/',
  'docs/GETTING-STARTED.md': 'docs/getting-started/',
  'docs/integrations/README.md': 'docs/providers/',
  'docs/API.md': 'docs/api/',
  'docs/ARCHITECTURE.md': 'docs/architecture/',
  'mcp/README.md': 'docs/mcp/',
};

function rewriteHref(href, dir) {
  if (!href || /^(?:[a-z][a-z0-9+.-]*:|\/\/)/i.test(href)) return { href, external: /^https?:/i.test(href ?? '') };
  if (href.startsWith('#')) return { href };
  const [pathPart, frag] = href.split('#');
  const repoPath = posix.normalize(posix.join(dir, pathPart));
  const hash = frag ? `#${frag}` : '';
  if (PAGE_OF[repoPath]) return { href: `${BASE}${PAGE_OF[repoPath]}${hash}` };
  const isDir = pathPart.endsWith('/') || !/\.[a-z0-9]+$/i.test(repoPath.split('/').pop() ?? '');
  return { href: `${REPO_URL}/${isDir ? 'tree' : 'blob'}/main/${repoPath}${hash}`, external: true };
}

const slugify = (text) =>
  text
    .toLowerCase()
    .replace(/<[^>]+>/g, '')
    .replace(/[^\p{L}\p{N}\s-]/gu, '')
    .trim()
    .replace(/\s+/g, '-');

function makeRenderer() {
  const ctx = { dir: '', slugs: new Map(), toc: [] };
  const md = new Marked({ gfm: true });
  md.use({
    renderer: {
      heading({ tokens, depth, text }) {
        const html = this.parser.parseInline(tokens);
        const base = slugify(text) || 'section';
        const n = ctx.slugs.get(base) ?? 0;
        ctx.slugs.set(base, n + 1);
        const id = n === 0 ? base : `${base}-${n}`;
        if (depth === 2 || depth === 3) ctx.toc.push({ depth, id, text: text.replace(/`/g, '') });
        const anchor = depth >= 2 ? `<a class="anchor" href="#${id}" aria-label="Link to this section">#</a>` : '';
        return `<h${depth} id="${id}">${html}${anchor}</h${depth}>\n`;
      },
      code({ text, lang }) {
        const language = (lang ?? '').trim().split(/\s+/)[0];
        if (language === 'mermaid') return `<pre class="mermaid">${esc(text)}</pre>\n`;
        if (language && hljs.getLanguage(language)) {
          const out = hljs.highlight(text, { language, ignoreIllegals: true }).value;
          return `<pre><code class="hljs language-${esc(language)}">${out}</code></pre>\n`;
        }
        return `<pre><code>${esc(text)}</code></pre>\n`;
      },
      table(token) {
        const emptyHead = token.header.every((c) => c.text.trim() === '');
        // Allow line breaks after slashes so long paths ("/posts/{id}/schedule") wrap instead of widening the table.
        const wrappable = (html) => html.split(/(<[^>]+>)/).map((seg) => (seg.startsWith('<') ? seg : seg.replaceAll('/', '/<wbr>'))).join('');
        const cell = (c) => `<${c.header ? 'th scope="col"' : 'td'}>${wrappable(this.parser.parseInline(c.tokens))}</${c.header ? 'th' : 'td'}>`;
        const head = emptyHead ? '' : `<thead><tr>${token.header.map(cell).join('')}</tr></thead>`;
        const body = token.rows.map((r) => `<tr>${r.map(cell).join('')}</tr>`).join('');
        return `<div class="table-wrap"><table>${head}<tbody>${body}</tbody></table></div>\n`;
      },
      link({ href, title, tokens }) {
        const r = rewriteHref(href, ctx.dir);
        const attrs = r.external ? ' rel="noopener"' : '';
        const t = title ? ` title="${esc(title)}"` : '';
        return `<a href="${esc(r.href ?? '')}"${t}${attrs}>${this.parser.parseInline(tokens)}</a>`;
      },
    },
  });
  return { md, ctx };
}

/** Lex a markdown file and split it into its H2 sections. Leading numbering ("2. Local setup") is dropped. */
function sectionsOf(file) {
  const { md } = makeRenderer();
  const tokens = md.lexer(read(join(repoRoot, file)));
  const sections = [];
  let current = { title: '', tokens: [] };
  for (const t of tokens) {
    if (t.type === 'heading' && t.depth === 2) {
      sections.push(current);
      t.text = t.text.replace(/^\d+\.\s+/, '');
      if (t.tokens?.[0]?.type === 'text') t.tokens[0].text = t.tokens[0].text.replace(/^\d+\.\s+/, '');
      current = { title: t.text, tokens: [t] };
    } else {
      current.tokens.push(t);
    }
  }
  sections.push(current);
  const withLinks = (list) => Object.assign(list, { links: tokens.links });
  return sections.map((s) => ({ ...s, tokens: withLinks(s.tokens) }));
}

const take = (sections, ...titles) => {
  const out = [];
  for (const title of titles) {
    const s = sections.find((x) => x.title === title);
    if (!s) fail(`README.md has no "## ${title}" section (docs pages are assembled from it)`);
    out.push(s);
  }
  return out;
};

/** Parse tokens with a fresh renderer so heading ids and the TOC are per page. */
function renderBlocks(blocks) {
  const { md, ctx } = makeRenderer();
  let html = '';
  for (const b of blocks) {
    ctx.dir = b.dir ?? '';
    const tokens = b.shift
      ? b.tokens.map((t) => (t.type === 'heading' ? { ...t, depth: Math.min(6, t.depth + b.shift) } : t))
      : b.tokens;
    html += md.parser(Object.assign([...tokens], { links: b.tokens.links }));
  }
  return { html, toc: ctx.toc };
}

// ------------------------------------------------------------------ docs pages

const readme = sectionsOf('README.md');
const R = (...t) => take(readme, ...t).map((s) => ({ tokens: s.tokens, dir: '' }));

/** A whole markdown file without its H1 (the page title comes from the config), rendered relative to its folder. */
function wholeFile(file) {
  const sections = sectionsOf(file);
  const tokens = sections.flatMap((s) => s.tokens).filter((t) => !(t.type === 'heading' && t.depth === 1));
  return { tokens: Object.assign(tokens, { links: sections[0].tokens.links }), dir: posix.dirname(file) === '.' ? '' : posix.dirname(file) };
}

const mcpServer = wholeFile('mcp/README.md');
const serverReference = { type: 'heading', depth: 1, raw: '# Server reference\n', text: 'Server reference', tokens: [{ type: 'text', raw: 'Server reference', text: 'Server reference' }] };

// The README's intro (wordmark, badges, hero image) is made for GitHub, so the overview starts at its first section.
const DOCS = [
  {
    slug: '',
    nav: 'Overview',
    title: 'Overview',
    description: 'What SocialOS is, what it does today, which networks it supports and how its parts fit together.',
    lead: 'Run SocialOS yourself, connect your accounts and let AI agents work with them through a scoped MCP server.',
    blocks: () => R('Why SocialOS', 'Features', 'Supported networks', 'Architecture'),
  },
  {
    slug: 'getting-started',
    nav: 'Getting started',
    title: 'Getting started',
    description: 'Run SocialOS locally with Docker, configure it, apply migrations and run the test suites.',
    lead: 'From a clean checkout to a running stack, with or without production credentials.',
    blocks: () => [wholeFile('docs/GETTING-STARTED.md')],
  },
  {
    slug: 'providers',
    nav: 'Providers & OAuth',
    title: 'Social providers and OAuth setup',
    description: 'What LinkedIn and Telegram support, how to add a provider and how to set up OAuth and the Telegram bot.',
    lead: 'Capabilities are reported per provider, honestly. Only LinkedIn and Telegram publish today.',
    blocks: () => [wholeFile('docs/integrations/README.md')],
  },
  {
    slug: 'api',
    nav: 'REST API',
    title: 'REST API',
    description: 'Endpoints, error format, post lifecycle and publishing idempotency of the SocialOS REST API.',
    lead: 'Everything the app and the MCP server do goes through this API. The full contract, including the database and scheduler, is in the architecture reference.',
    blocks: () => [wholeFile('docs/API.md')],
  },
  {
    slug: 'mcp',
    nav: 'MCP server',
    title: 'MCP server',
    description: 'The SocialOS MCP tools, their scopes and risk levels, and how to configure Claude Code, Cursor, Claude Desktop and other clients.',
    lead: 'Agents connect with a scoped, revocable API key. Tools a key has no scope for are not listed, and every call is checked again by the REST API.',
    blocks: () => [...R('Use with your AI agent'), { ...mcpServer, tokens: Object.assign([serverReference, ...mcpServer.tokens], { links: mcpServer.tokens.links }), shift: 1 }],
  },
  {
    slug: 'architecture',
    nav: 'Architecture',
    title: 'Architecture and contract',
    description: 'The single source of truth: services, domain rules, database schema, REST contract, MCP tools, scheduler and OAuth flow.',
    lead: 'The reference for how the pieces fit: services, the database, the REST contract, MCP tools and the publishing flow.',
    blocks: () => [wholeFile('docs/ARCHITECTURE.md')],
  },
];

// ------------------------------------------------------------------ templates

// Docs and the 404 share a calm layout; the landing page has its own (full-bleed hero, floating nav, big footer).
const layout = read(join(src, 'layout.html'));
const landingLayout = read(join(src, 'layout-landing.html'));

function page({ path, title, description, content, docs = false, landing = false }) {
  const canonical = `${SITE_URL}${BASE}${path}`;
  return (landing ? landingLayout : layout)
    .replaceAll('{{title}}', esc(title))
    .replaceAll('{{description}}', esc(description))
    .replaceAll('{{canonical}}', canonical)
    .replaceAll('{{siteUrl}}', SITE_URL)
    .replaceAll('{{repoUrl}}', REPO_URL)
    .replaceAll('{{navDocs}}', docs ? ' aria-current="page"' : '')
    .replace('{{content}}', () => content)
    .replaceAll('{{base}}', BASE);
}

function write(rel, data) {
  const file = join(dist, rel);
  mkdirSync(dirname(file), { recursive: true });
  writeFileSync(file, data);
}

/** `{{shot:name:alt}}` -> a <picture> with light and dark variants. */
function shots(html) {
  return html.replace(/\{\{shot:(\w+):(.*?)\}\}/g, (_, name, alt) => {
    const dir = join(src, 'assets/screens');
    if (!existsSync(join(dir, `${name}-light.png`))) fail(`missing screenshot assets/screens/${name}-light.png (run \`npm run screenshots\`)`);
    const dark = existsSync(join(dir, `${name}-dark.png`));
    const [w, h] = pngSize(join(dir, `${name}-light.png`));
    return (
      `<picture>${dark ? `<source media="(prefers-color-scheme: dark)" srcset="${BASE}assets/screens/${name}-dark.png">` : ''}` +
      `<img src="${BASE}assets/screens/${name}-light.png" width="${w}" height="${h}" alt="${esc(alt)}" loading="lazy" decoding="async"></picture>`
    );
  });
}

/** `{{brand:name}}` -> the network's monochrome mark (Simple Icons, CC0), inlined so it takes the text colour. */
function brands(html) {
  return html.replace(/\{\{brand:(\w+)\}\}/g, (_, name) => {
    const file = join(src, `assets/brands/${name}.svg`);
    if (!existsSync(file)) fail(`missing brand mark assets/brands/${name}.svg`);
    const d = read(file).match(/ d="([^"]+)"/)?.[1];
    return `<svg class="brand-ico" viewBox="0 0 24 24" aria-hidden="true" focusable="false"><path fill="currentColor" d="${d}"/></svg>`;
  });
}

/** How many networks the README marks as live, so the page never claims a number the docs do not. */
function liveNetworkCount() {
  const table = take(readme, 'Supported networks')[0].tokens.find((t) => t.type === 'table');
  const n = table ? table.rows.filter((r) => /\bLive\b/.test(r[1].text)).length : 0;
  if (n < 2) fail('README.md "Supported networks" has no rows marked Live');
  return n;
}

function pngSize(file) {
  const b = readFileSync(file);
  return [b.readUInt32BE(16), b.readUInt32BE(20)];
}

// ------------------------------------------------------------------ MCP tools table (from the README)

function mcpToolsTable() {
  const section = take(readme, 'Use with your AI agent')[0];
  const table = section.tokens.find((t) => t.type === 'table');
  if (!table) fail('README.md "Use with your AI agent" has no tool table');
  const { md } = makeRenderer();
  const renderCell = (c) => md.parseInline(c.text);
  let count = 0;
  const rows = table.rows
    .map((r) => {
      const [tool, scope, risk] = r;
      count += (tool.text.match(/`/g) ?? []).length / 2;
      const kind = /critical/i.test(risk.text) ? 'critical' : /sensitive/i.test(risk.text) ? 'sensitive' : /medium/i.test(risk.text) ? 'medium' : 'safe';
      return `<tr><td>${renderCell(tool)}</td><td>${renderCell(scope)}</td><td><span class="risk risk-${kind}">${renderCell(risk)}</span></td></tr>`;
    })
    .join('');
  if (count < 10) fail(`expected at least 10 MCP tools in the README table, found ${count}`);
  return {
    count,
    html: `<table class="mcp-tools"><thead><tr><th scope="col">Tool</th><th scope="col">Scope</th><th scope="col">Risk</th></tr></thead><tbody>${rows}</tbody></table>`,
  };
}

// ------------------------------------------------------------------ build

rmSync(dist, { recursive: true, force: true });
mkdirSync(dist, { recursive: true });
cpSync(join(src, 'assets'), join(dist, 'assets'), { recursive: true });

// Design tokens: the app's file is the single source. The app switches theme with a `.dark` class, the site follows the
// OS, so the `.dark` block becomes a prefers-color-scheme rule on :root.
{
  const css = read(join(repoRoot, 'frontend/src/styles/tokens.css'));
  const dark = css.match(/^\.dark \{([\s\S]*?)^\}/m);
  if (!dark || !/^:root \{/m.test(css)) fail('frontend/src/styles/tokens.css must have a :root block and a .dark block');
  write('assets/tokens.css', `${css.slice(0, dark.index)}@media (prefers-color-scheme: dark) {\n  :root {${dark[1].replace(/\n(?=.)/g, '\n  ')}  }\n}\n`);
}

// Landing page
{
  const tools = mcpToolsTable();
  const content = brands(shots(read(join(src, 'pages/index.html'))))
    .replace('{{mcpTools}}', () => tools.html)
    .replaceAll('{{toolCount}}', String(tools.count))
    .replaceAll('{{liveCount}}', String(liveNetworkCount()))
    .replaceAll('{{base}}', BASE);
  write(
    'index.html',
    page({
      path: '',
      title: 'SocialOS: publish to social networks, for you and your AI agents',
      description: 'Connect your social accounts, compose once, schedule per platform, and let AI agents help through a scoped MCP server. Try the demo in your browser.',
      content,
      landing: true,
    }),
  );
}

// Docs
const pages = [];
for (const d of DOCS) {
  const { html, toc } = renderBlocks(d.blocks());
  const url = `docs/${d.slug ? `${d.slug}/` : ''}`;
  pages.push({ ...d, url, html, toc });
}
pages.forEach((p, i) => {
  const nav = pages
    .map((x) => `<li><a href="${BASE}${x.url}"${x === p ? ' aria-current="page"' : ''}>${esc(x.nav)}</a></li>`)
    .join('');
  const toc = p.toc.length
    ? `<aside class="docs-toc" aria-label="On this page"><h2>On this page</h2><ul>${p.toc
        .filter((t) => t.depth === 2)
        .map((t) => `<li class="l${t.depth}"><a href="#${t.id}">${esc(t.text)}</a></li>`)
        .join('')}</ul></aside>`
    : '<aside class="docs-toc"></aside>';
  const prev = pages[i - 1];
  const next = pages[i + 1];
  const pager = `<nav class="pager" aria-label="Pagination"><span>${prev ? `<a href="${BASE}${prev.url}"><small>Previous</small>${esc(prev.nav)}</a>` : ''}</span><span style="text-align:right">${next ? `<a href="${BASE}${next.url}"><small>Next</small>${esc(next.nav)}</a>` : ''}</span></nav>`;
  const cards =
    p.slug === ''
      ? `<ul class="doc-cards">${pages
          .slice(1)
          .map((x) => `<li><a href="${BASE}${x.url}"><strong>${esc(x.nav)}</strong><span>${esc(x.description)}</span></a></li>`)
          .join('')}</ul>`
      : '';
  // The page's own H1 comes from the config; the README/ARCHITECTURE H1 was removed above.
  const content = `    <div class="wrap docs">
      <nav class="docs-nav" aria-label="Documentation"><details open><summary>Documentation</summary><h2>Documentation</h2><ul>${nav}</ul></details></nav>
      <article class="prose">
        <h1>${esc(p.title)}</h1>
        <p class="lead">${esc(p.lead)}</p>
        ${cards}
        ${p.html}
        ${pager}
      </article>
      ${toc}
    </div>`;
  write(`${p.url}index.html`, page({ path: p.url, title: `${p.title} · SocialOS docs`, description: p.description, content, docs: true }));
});

// 404, robots, sitemap
write('404.html', page({ path: '404.html', title: 'Page not found · SocialOS', description: 'This page does not exist.', content: read(join(src, 'pages/404.html')).replaceAll('{{base}}', BASE) }));
write('robots.txt', `User-agent: *\nAllow: /\nSitemap: ${SITE_URL}${BASE}sitemap.xml\n`);
write(
  'sitemap.xml',
  `<?xml version="1.0" encoding="UTF-8"?>\n<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n${['', ...pages.map((p) => p.url), 'demo/']
    .map((u) => `  <url><loc>${SITE_URL}${BASE}${u}</loc></url>`)
    .join('\n')}\n</urlset>\n`,
);
write('.nojekyll', '');

// Demo
if (WITH_DEMO) {
  if (!existsSync(join(DEMO_DIR, 'index.html'))) {
    fail(`no demo export at ${DEMO_DIR}. Build it first: (cd frontend && npm run build:demo), or pass --no-demo.`);
  }
  cpSync(DEMO_DIR, join(dist, 'demo'), { recursive: true });
  // The demo bakes its base path in at build time; it must match where it is published.
  const probe = read(join(dist, 'demo/index.html'));
  if (!probe.includes(`${BASE}demo/_next/`)) {
    fail(`the demo export was not built for the base path "${BASE}demo". Rebuild it with NEXT_PUBLIC_BASE_PATH=${BASE}demo (or set SITE_BASE).`);
  }
}

const size = (dir) => readdirSync(dir, { withFileTypes: true }).reduce((n, e) => n + (e.isDirectory() ? size(join(dir, e.name)) : statSync(join(dir, e.name)).size), 0);
console.log(`site build: ${pages.length + 2} pages${WITH_DEMO ? ' + demo' : ''} -> ${dist} (${(size(dist) / 1024 / 1024).toFixed(1)} MB), base ${BASE}`);
