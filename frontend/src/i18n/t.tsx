'use client';
import type { Values } from '@/i18n/translate';
import type { KeysOf } from '@/i18n/translate';
import { useTranslations } from '@/i18n/use-translations';

/**
 * A translated string for server components, which cannot call hooks: `<PageHeader title={<T k="dashboard.title" />} />`.
 * Client components should use `useTranslations` directly.
 */
export function T({ k, values }: { k: KeysOf<undefined>; values?: Values }) {
  const t = useTranslations();
  return <>{t(k, values)}</>;
}
