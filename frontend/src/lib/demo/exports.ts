/** Demo stand-in for the account export endpoints (D-018): the same states and rules, on plain data. */
import type { DataExport } from '../types';
import { seedId } from './ids';
import type { DemoResponse, DemoState } from './model';

const MIN = 60_000;
const DAY = 86_400_000;
/** The demo "builds" the archive in a few seconds so the preparing state is visible. */
const BUILD_MS = 6000;
const RETENTION_MS = 7 * DAY;
const COOLDOWN_MS = DAY;

const body = (status: number, b?: unknown): DemoResponse => ({ status, body: b });
const error = (status: number, code: string, message: string): DemoResponse => ({ status, body: { error: { code, message, request_id: 'demo' } } });

/** What the list shows at `nowMs`: a pending export becomes ready after BUILD_MS, a ready one expires with its retention. */
export function visibleExports(s: DemoState, nowMs: number): DataExport[] {
  return (s.exports ?? [])
    .map((stored): DataExport => {
      let e = stored;
      if (e.status === 'pending' && nowMs - Date.parse(e.created_at) >= BUILD_MS) {
        e = { ...e, status: 'ready', size_bytes: demoArchive(s).length, expires_at: new Date(Date.parse(e.created_at) + RETENTION_MS).toISOString() };
      }
      return e.status === 'ready' && e.expires_at && Date.parse(e.expires_at) <= nowMs ? { ...e, status: 'expired' } : e;
    })
    .sort((a, b) => b.created_at.localeCompare(a.created_at));
}

/** The "archive" the demo hands out: a JSON file with the demo's own data, never any credential. */
function demoArchive(s: DemoState): string {
  return JSON.stringify({ note: 'Demo export. The real one is a ZIP with more files.', profile: { email: s.user.email, display_name: s.user.display_name }, social_accounts: s.accounts, posts: s.posts.map((p) => ({ ...p, settle_at: undefined })), audit_logs: s.audit }, null, 2);
}

/** Handles `/account/exports`; returns null for any other path so the engine carries on. `persist` stores the new export. */
export function handleExports(s: DemoState, method: string, path: string, nowMs: number, persist: () => void): DemoResponse | null {
  const m = path.match(/^\/account\/exports(?:\/([^/]+))?$/);
  if (!m) return null;
  const list = visibleExports(s, nowMs);
  if (!m[1] && method === 'GET') return body(200, { items: list });
  if (!m[1] && method === 'POST') {
    const last = list[0];
    if (last && (last.status === 'pending' || last.status === 'running')) return error(409, 'CONFLICT', 'an export is already being prepared');
    if (last?.status === 'ready' && nowMs - Date.parse(last.created_at) < COOLDOWN_MS) {
      return error(429, 'RATE_LIMITED', 'you exported your data less than 24 hours ago; download that export or try again later');
    }
    const e: DataExport = { id: seedId('e1000000', (s.exports?.length ?? 0) + 1), status: 'pending', size_bytes: 0, error_code: null, created_at: new Date(nowMs).toISOString(), expires_at: null };
    s.exports = [...list.map((x) => (x.status === 'ready' ? { ...x, status: 'expired' as const } : x)), e];
    persist();
    return body(202, e);
  }
  if (m[1] && method === 'GET') {
    const e = list.find((x) => x.id === m[1]);
    if (!e) return error(404, 'NOT_FOUND', 'export not found');
    if (e.status !== 'ready') return error(409, 'CONFLICT', 'this export is not available for download');
    return body(200, { ...e, url: `data:application/json;charset=utf-8,${encodeURIComponent(demoArchive(s))}`, url_expires_at: new Date(nowMs + 5 * MIN).toISOString() });
  }
  return error(404, 'NOT_FOUND', 'Not found');
}
