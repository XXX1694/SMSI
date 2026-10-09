'use client';
import { useMemo } from 'react';
import { useLocaleSettings } from '@/i18n/locale-provider';
import en from '@/i18n/en-all';
import { createTranslator, type KeysOf, type Namespace, type Translator } from '@/i18n/translate';

/** `const t = useTranslations('nav'); t('dashboard')`. Keys are typed from messages/en/*.json. Same shape as next-intl. */
export function useTranslations<N extends Namespace | undefined = undefined>(ns?: N): Translator<KeysOf<N>> {
  const { locale, messages, timeZone } = useLocaleSettings();
  return useMemo(() => createTranslator({ locale, messages, timeZone, fallback: en }, ns), [locale, messages, timeZone, ns]);
}
