/** @type {import('next').NextConfig} */

// Demo build (`npm run build:demo`): a static export that runs the whole app in the browser against
// an in-memory mock API. It is served from a sub-path (GitHub Pages: /SMSI/demo) and never proxies.
const demo = process.env.NEXT_PUBLIC_DEMO === 'true';
// An empty NEXT_PUBLIC_BASE_PATH means "serve at the domain root".
const demoBasePath = (process.env.NEXT_PUBLIC_BASE_PATH ?? '/SMSI/demo').replace(/\/+$/, '');

const demoConfig = {
  output: 'export',
  // With `output: 'export'` the static site is written to distDir (build intermediates still use .next).
  distDir: 'out',
  basePath: demoBasePath,
  assetPrefix: demoBasePath || undefined,
  // Static hosts serve /dashboard/ from dashboard/index.html without any rewrite rules.
  trailingSlash: true,
  images: { unoptimized: true },
  reactStrictMode: true,
  poweredByHeader: false,
  env: { NEXT_PUBLIC_DEMO: 'true', NEXT_PUBLIC_BASE_PATH: demoBasePath },
};

const apiInternal = process.env.API_INTERNAL_URL || 'http://localhost:8080';

const productionConfig = {
  output: 'standalone',
  reactStrictMode: true,
  poweredByHeader: false,
  // Inlined at build time so the demo-only code paths are removed from the normal bundle.
  env: { NEXT_PUBLIC_DEMO: 'false' },
  async rewrites() {
    // Same-origin proxy so session cookies are first-party.
    return [{ source: '/api/v1/:path*', destination: `${apiInternal}/api/v1/:path*` }];
  },
};

export default demo ? demoConfig : productionConfig;
