'use client';
import { useCallback, useEffect, useState } from 'react';
import { ApiError } from '@/lib/api';
import { DEMO, DEMO_CHANGE_EVENT } from '@/lib/demo/config';

export interface AsyncState<T> {
  data: T | null;
  error: ApiError | null;
  loading: boolean;
  reload: () => void;
}

/** Runs `fn` on mount and whenever `fn` identity changes (wrap it in useCallback). */
export function useAsync<T>(fn: () => Promise<T>): AsyncState<T> {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<ApiError | null>(null);
  const [loading, setLoading] = useState(true);
  const [tick, setTick] = useState(0);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    fn().then(
      (d) => {
        if (cancelled) return;
        setData(d);
        setError(null);
        setLoading(false);
      },
      (e: unknown) => {
        if (cancelled) return;
        setError(e instanceof ApiError ? e : new ApiError(0, 'UNKNOWN', 'Something went wrong.'));
        setLoading(false);
      },
    );
    return () => {
      cancelled = true;
    };
  }, [fn, tick]);

  // Demo only: reload when the simulated scheduler published something in the background.
  useEffect(() => {
    if (!DEMO) return undefined;
    const onChange = () => setTick((t) => t + 1);
    window.addEventListener(DEMO_CHANGE_EVENT, onChange);
    return () => window.removeEventListener(DEMO_CHANGE_EVENT, onChange);
  }, []);

  const reload = useCallback(() => setTick((t) => t + 1), []);
  return { data, error, loading, reload };
}

export function errorMessage(e: unknown): string {
  if (e instanceof ApiError) return e.requestId ? `${e.message} (ref ${e.requestId})` : e.message;
  return e instanceof Error ? e.message : 'Something went wrong.';
}
