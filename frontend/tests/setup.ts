import '@testing-library/jest-dom/vitest';
import { cleanup } from '@testing-library/react';
import { afterEach } from 'vitest';

// Node 25+ ships an experimental global `localStorage` / `sessionStorage`. Vitest's jsdom environment never overwrites
// a global that already exists, so `window.localStorage` (window === globalThis here) becomes Node's stub, which has no
// working `setItem` / `clear` (and prints a --localstorage-file warning), instead of jsdom's Storage. Put jsdom's
// storages back so the tests behave the same on Node 22 (CI) and Node 25+. On Node 22 this changes nothing: the global
// already is jsdom's.
const dom = (globalThis as { jsdom?: { window: Pick<Window, 'localStorage' | 'sessionStorage'> } }).jsdom;
if (dom) {
  for (const name of ['localStorage', 'sessionStorage'] as const) {
    Object.defineProperty(globalThis, name, { value: dom.window[name], configurable: true, writable: true });
  }
}

afterEach(() => cleanup());
