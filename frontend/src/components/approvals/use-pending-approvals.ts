'use client';
import { useCallback, useEffect, useState } from 'react';
import { api } from '@/lib/api';

const CHANGED = 'socialos:approvals-changed';
const POLL_MS = 60_000;

/** Tells the sidebar badge to re-count after the owner decided something on the Approvals page. */
export function notifyApprovalsChanged(): void {
  window.dispatchEvent(new Event(CHANGED));
}

/**
 * How many approvals wait for the owner (null until known or when the count cannot be read: a badge that is
 * missing is better than a wrong one). Re-counted when the tab regains focus, every minute and after a decision.
 */
export function usePendingApprovals(): number | null {
  const [count, setCount] = useState<number | null>(null);
  const refresh = useCallback(() => {
    api.approvals.list('pending', 100).then(
      (p) => setCount(p.items.length),
      () => setCount(null),
    );
  }, []);
  useEffect(() => {
    refresh();
    const timer = window.setInterval(refresh, POLL_MS);
    window.addEventListener('focus', refresh);
    window.addEventListener(CHANGED, refresh);
    return () => {
      window.clearInterval(timer);
      window.removeEventListener('focus', refresh);
      window.removeEventListener(CHANGED, refresh);
    };
  }, [refresh]);
  return count;
}
