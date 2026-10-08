import { existsSync, readdirSync } from 'node:fs';
import { chromium } from 'playwright-core';

/** CHROMIUM_PATH, a preinstalled /opt/pw-browsers chromium, or a system Chrome (CI runners have one). */
export function findChromium() {
  if (process.env.CHROMIUM_PATH) return process.env.CHROMIUM_PATH;
  const root = '/opt/pw-browsers';
  if (existsSync(root)) {
    for (const d of readdirSync(root).filter((x) => x.startsWith('chromium-')).sort().reverse()) {
      const p = `${root}/${d}/chrome-linux/chrome`;
      if (existsSync(p)) return p;
    }
  }
  for (const p of ['/usr/bin/google-chrome', '/usr/bin/google-chrome-stable', '/usr/bin/chromium', '/usr/bin/chromium-browser']) {
    if (existsSync(p)) return p;
  }
  return undefined;
}

export function launch() {
  const proxy = process.env.HTTPS_PROXY || process.env.https_proxy;
  return chromium.launch({
    executablePath: findChromium(),
    args: ['--no-sandbox'],
    // Local pages are served from 127.0.0.1; only external requests (the mermaid CDN) use the proxy.
    ...(process.env.PW_USE_PROXY && proxy ? { proxy: { server: proxy, bypass: '127.0.0.1,localhost' } } : {}),
  });
}
