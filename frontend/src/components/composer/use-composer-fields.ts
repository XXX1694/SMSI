'use client';
import { useCallback, useMemo, useState } from 'react';
import { usePrefs } from '@/components/prefs-provider';
import type { ComposerState } from '@/lib/composer';
import type { FormValues } from '@/lib/post-edit';
import { zonedToUtcIso } from '@/lib/time';

/** The composer's editable values plus the derived `ComposerState` the validators and previews read. */
export function useComposerFields(initial: FormValues) {
  const { timezone } = usePrefs();
  const [form, setForm] = useState<FormValues>(initial);
  const patch = useCallback((p: Partial<FormValues>) => setForm((cur) => ({ ...cur, ...p })), []);
  const scheduledAtUtc = useMemo(() => (form.date ? zonedToUtcIso(form.date, form.time, timezone) : null), [form.date, form.time, timezone]);
  const state: ComposerState = { content: form.content, overrides: form.overrides, accountIds: form.accountIds, media: form.media, scheduledAtUtc };
  return { form, setForm, patch, state, timezone };
}

export type ComposerFields = ReturnType<typeof useComposerFields>;
