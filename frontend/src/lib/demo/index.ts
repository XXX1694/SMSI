/**
 * Browser glue for the demo backend: one engine per page, persisted to localStorage, plus a
 * background tick so scheduled posts publish while the tab is open. Imported lazily by the API
 * client, only in `NEXT_PUBLIC_DEMO=true` builds.
 */
import { DEMO_CHANGE_EVENT } from './config';
import { DemoEngine } from './engine';
import type { DemoRequest, DemoResponse } from './model';
import { STORAGE_KEY, browserStorage, clearState, loadState, saveState } from './store';

const TICK_MS = 5000;
const LATENCY_MS = 90;

let engine: DemoEngine | null = null;
let ticker: ReturnType<typeof setInterval> | null = null;

function getEngine(): DemoEngine {
  if (engine) return engine;
  const storage = browserStorage();
  engine = new DemoEngine(loadState(storage), { onChange: (s) => saveState(storage, s) });
  if (typeof window !== 'undefined') {
    if (!ticker) {
      ticker = setInterval(() => {
        if (engine?.advance()) window.dispatchEvent(new Event(DEMO_CHANGE_EVENT));
      }, TICK_MS);
      // Another tab changed the demo: pick its state up on the next request.
      window.addEventListener('storage', (e) => {
        if (e.key === STORAGE_KEY || e.key === null) engine = null;
      });
    }
  }
  return engine;
}

/** Make an uploaded image small enough to keep in localStorage. */
async function thumbnail(file: File): Promise<string | undefined> {
  try {
    if (typeof createImageBitmap !== 'function' || typeof document === 'undefined') return undefined;
    const bmp = await createImageBitmap(file);
    const scale = Math.min(1, 480 / Math.max(bmp.width, bmp.height));
    const canvas = document.createElement('canvas');
    canvas.width = Math.max(1, Math.round(bmp.width * scale));
    canvas.height = Math.max(1, Math.round(bmp.height * scale));
    const ctx = canvas.getContext('2d');
    if (!ctx) return undefined;
    ctx.fillStyle = '#fff';
    ctx.fillRect(0, 0, canvas.width, canvas.height);
    ctx.drawImage(bmp, 0, 0, canvas.width, canvas.height);
    return canvas.toDataURL('image/jpeg', 0.72);
  } catch {
    return undefined;
  }
}

async function describeUpload(form: FormData): Promise<DemoRequest['upload']> {
  const f = form.get('file');
  if (!(f instanceof File)) return undefined;
  return { name: f.name, mime: f.type, size: f.size, url: f.type.startsWith('image/') ? await thumbnail(f) : undefined };
}

export interface DemoFetchInit {
  method: DemoRequest['method'];
  path: string;
  query?: DemoRequest['query'];
  body?: unknown;
  form?: FormData;
}

/** The demo's stand-in for `fetch('/api/v1' + path)`: same status codes and JSON bodies. */
export async function demoFetch(init: DemoFetchInit): Promise<DemoResponse> {
  const upload = init.form ? await describeUpload(init.form) : undefined;
  if (init.method !== 'GET') await new Promise((r) => setTimeout(r, LATENCY_MS));
  const res = getEngine().handle({ method: init.method, path: init.path, query: init.query, body: init.body, upload });
  // Round-trip through JSON like a real response, so the UI can never mutate the demo's state.
  return { status: res.status, body: res.body === undefined ? undefined : (JSON.parse(JSON.stringify(res.body)) as unknown) };
}

/** Throw the demo's data away; the next request seeds a fresh copy. */
export function resetDemo(): void {
  clearState(browserStorage());
  engine = null;
}
