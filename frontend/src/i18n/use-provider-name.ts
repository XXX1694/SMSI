'use client';
import { useCallback } from 'react';
import { useTranslations } from '@/i18n/use-translations';
import { providerName } from '@/lib/format';

/** `providerName` bound to the active locale: the display name of a network id, "Unknown" for an empty one. */
export function useProviderName(): (id: string) => string {
  const t = useTranslations();
  return useCallback((id: string) => providerName(id, t), [t]);
}
