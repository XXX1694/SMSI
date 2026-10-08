import { Checkbox } from '@/components/ui/checkbox';
import { Label } from '@/components/ui/input';

/** Opt-in on password reset and change: sessions are always signed out, API keys and MCP connections are not. */
export function RevokeKeysOption({ id, checked, onChange }: { id: string; checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <div className="flex items-start gap-2">
      <Checkbox id={id} checked={checked} onCheckedChange={(c) => onChange(c === true)} />
      <div className="space-y-0.5">
        <Label htmlFor={id} className="font-normal">
          Also revoke all API keys and MCP connections
        </Label>
        <p className="text-xs text-muted-foreground">Otherwise they keep working and you should review them yourself.</p>
      </div>
    </div>
  );
}
