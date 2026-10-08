import { STORAGE_KEY } from './config';
import type { DemoState } from './model';
import { buildSeed } from './seed';

export { STORAGE_KEY };
/** Seeded dates are relative to the first visit, so an old copy is replaced by a fresh one. */
export const MAX_AGE_MS = 14 * 86_400_000;

export type StorageLike = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>;

/** `window.localStorage` or null when the browser blocks it (private mode, disabled storage). */
export function browserStorage(): StorageLike | null {
  try {
    return typeof window === 'undefined' ? null : window.localStorage;
  } catch {
    return null;
  }
}

function isState(v: unknown): v is DemoState {
  if (typeof v !== 'object' || v === null) return false;
  const r = v as Record<string, unknown>;
  const u = r.user as Record<string, unknown> | undefined;
  return (
    r.version === 1 &&
    typeof r.seeded_at === 'string' &&
    typeof r.signed_in === 'boolean' &&
    typeof u === 'object' &&
    u !== null &&
    typeof u.email === 'string' &&
    ['accounts', 'posts', 'media', 'api_keys', 'mcp_connections', 'audit', 'links'].every((k) => Array.isArray(r[k])) &&
    typeof r.usage === 'object' &&
    r.usage !== null
  );
}

/** Restore the saved demo, or start from a fresh seed when nothing usable is stored. */
export function loadState(storage: StorageLike | null, now: Date = new Date()): DemoState {
  try {
    const raw = storage?.getItem(STORAGE_KEY);
    if (raw) {
      const parsed: unknown = JSON.parse(raw);
      if (isState(parsed) && now.getTime() - Date.parse(parsed.seeded_at) < MAX_AGE_MS) return parsed;
    }
  } catch {
    /* corrupted or blocked: fall through to a fresh seed */
  }
  return buildSeed(now);
}

export function saveState(storage: StorageLike | null, state: DemoState): void {
  try {
    storage?.setItem(STORAGE_KEY, JSON.stringify(state));
  } catch {
    /* quota exceeded or blocked: the demo keeps working from memory */
  }
}

export function clearState(storage: StorageLike | null): void {
  try {
    storage?.removeItem(STORAGE_KEY);
  } catch {
    /* ignore */
  }
}
