import { CheckboxField } from '@/components/ui/checkbox';

/** Opt-in on password reset and change: sessions are always signed out, API keys and MCP connections are not. */
export function RevokeKeysOption({ id, checked, onChange }: { id: string; checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <CheckboxField
      id={id}
      checked={checked}
      onCheckedChange={(c) => onChange(c === true)}
      label="Also revoke all API keys and MCP connections"
      description="If you leave this off, they keep working."
    />
  );
}
