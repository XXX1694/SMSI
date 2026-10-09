'use client';
import { useCallback, useEffect, useState } from 'react';
import { ApiError } from '@/lib/api';
import { DEMO, DEMO_CHANGE_EVENT } from '@/lib/demo/config';
import { enErrorsT } from '@/i18n/en';
import { useTranslations } from '@/i18n/use-translations';
import { describeErrorCode, friendlyMessage } from '@/lib/errors';
import type { AppT } from '@/i18n/translate';

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
        setError(e instanceof ApiError ? e : new ApiError(0, 'UNKNOWN', describeErrorCode('UNKNOWN', enErrorsT)));
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

/** The sentence for any thrown value, with the support reference of an API error when there is one. */
export function errorMessage(e: unknown, t: AppT, withRef = true): string {
  if (e instanceof ApiError) {
    const text = friendlyMessage(e.code, e.message, t);
    return withRef && e.requestId ? t('errors.withRef', { text, id: e.requestId }) : text;
  }
  return friendlyMessage(null, e instanceof Error ? e.message : null, t);
}

/** `errorMessage` bound to the active language: `const errorText = useErrorText(); errorText(err)`. */
export function useErrorText(): (e: unknown, withRef?: boolean) => string {
  const t = useTranslations();
  return useCallback((e: unknown, withRef = true) => errorMessage(e, t, withRef), [t]);
}
