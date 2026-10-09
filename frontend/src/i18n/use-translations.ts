'use client';
import { useMemo } from 'react';
import { useLocaleSettings } from '@/i18n/locale-provider';
import { createTranslator, type KeysOf, type Namespace, type Translator } from '@/i18n/translate';

/** `const t = useTranslations('nav'); t('dashboard')`. Keys are typed from messages/en/*.json. Same shape as next-intl. */
export function useTranslations<N extends Namespace | undefined = undefined>(ns?: N): Translator<KeysOf<N>> {
  const { locale, messages, english, timeZone } = useLocaleSettings();
  return useMemo(() => createTranslator({ locale, messages, timeZone, fallback: english }, ns), [locale, messages, english, timeZone, ns]);
}
