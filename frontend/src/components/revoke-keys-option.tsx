'use client';
import { CheckboxField } from '@/components/ui/checkbox';
import { useTranslations } from '@/i18n/use-translations';

/** Opt-in on password reset and change: sessions are always signed out, API keys and MCP connections are not. */
export function RevokeKeysOption({ id, checked, onChange }: { id: string; checked: boolean; onChange: (v: boolean) => void }) {
  const t = useTranslations('auth');
  return (
    <CheckboxField
      id={id}
      checked={checked}
      onCheckedChange={(c) => onChange(c === true)}
      label={t('revokeKeys')}
      description={t('revokeKeysHint')}
    />
  );
}
