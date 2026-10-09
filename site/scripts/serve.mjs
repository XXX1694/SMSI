#!/usr/bin/env node
/**
 * Minimal static server that mimics GitHub Pages for a project site: files under `dir` are
 * served below `base` (default /steerpost/), directories fall back to index.html and unknown paths
 * return 404.html with status 404.
 *
 *   node scripts/serve.mjs [dir=dist] [port=4173] [base=/steerpost/]
 */
import { createServer } from 'node:http';
import { existsSync, readFileSync, statSync } from 'node:fs';
import { extname, join, normalize, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const TYPES = {
  '.html': 'text/html; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.mjs': 'text/javascript; charset=utf-8',
  '.json': 'application/json; charset=utf-8',
  '.txt': 'text/plain; charset=utf-8',
  '.svg': 'image/svg+xml',
  '.png': 'image/png',
  '.webp': 'image/webp',
  '.woff2': 'font/woff2',
  '.ico': 'image/x-icon',
  '.map': 'application/json',
};

export function startServer({ dir = 'dist', port = 4173, base = '/steerpost/' } = {}) {
  const root = resolve(dir);
  const prefix = base.endsWith('/') ? base : `${base}/`;
  const server = createServer((req, res) => {
    const url = new URL(req.url ?? '/', 'http://localhost');
    let rel = decodeURIComponent(url.pathname);
    const send = (status, file) => {
      res.writeHead(status, { 'Content-Type': TYPES[extname(file)] ?? 'application/octet-stream', 'Cache-Control': 'no-store' });
      res.end(readFileSync(file));
    };
    if (rel === prefix.slice(0, -1)) {
      res.writeHead(301, { Location: prefix });
      return res.end();
    }
    if (!rel.startsWith(prefix)) return notFound();
    rel = rel.slice(prefix.length);
    let file = normalize(join(root, rel));
    if (!file.startsWith(root)) return notFound();
    if (existsSync(file) && statSync(file).isDirectory()) {
      if (!url.pathname.endsWith('/')) {
        res.writeHead(301, { Location: `${url.pathname}/${url.search}` });
        return res.end();
      }
      file = join(file, 'index.html');
    }
    if (existsSync(file) && statSync(file).isFile()) return send(200, file);
    return notFound();

    function notFound() {
      const nf = join(root, '404.html');
      if (existsSync(nf)) return send(404, nf);
      res.writeHead(404, { 'Content-Type': 'text/plain' });
      res.end('not found');
    }
  });
  return new Promise((resolveStart) => server.listen(port, '127.0.0.1', () => resolveStart(server)));
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const [dir = 'dist', port = '4173', base = '/steerpost/'] = process.argv.slice(2);
  await startServer({ dir, port: Number(port), base });
  console.log(`serving ${resolve(dir)} at http://127.0.0.1:${port}${base}`);
}
