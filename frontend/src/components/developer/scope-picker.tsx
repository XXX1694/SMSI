'use client';
import { Checkbox } from '@/components/ui/checkbox';
import { Notice } from '@/components/states';
import { groupScopes, RISK_LABEL, RISK_ORDER, hasDangerous } from '@/lib/scopes';

export function ScopePicker({ value, onChange }: { value: string[]; onChange: (v: string[]) => void }) {
  const groups = groupScopes();
  const toggle = (s: string, on: boolean) => onChange(on ? [...value, s] : value.filter((x) => x !== s));
  return (
    <div className="space-y-4">
      {RISK_ORDER.map((risk) => (
        <fieldset key={risk} className="space-y-2">
          <legend className="mb-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground">{RISK_LABEL[risk]}</legend>
          {groups[risk].map((s) => {
            const id = `scope-${s.scope}`;
            return (
              <div key={s.scope} className="flex items-start gap-2.5">
                <Checkbox id={id} checked={value.includes(s.scope)} onCheckedChange={(c) => toggle(s.scope, c === true)} />
                <label htmlFor={id} className="text-sm">
                  <span className="font-medium">{s.label}</span> <code className="text-xs text-muted-foreground">{s.scope}</code>
                  <span className="block text-xs text-muted-foreground">{s.description}</span>
                </label>
              </div>
            );
          })}
        </fieldset>
      ))}
      {hasDangerous(value) ? (
        <Notice>Dangerous scopes let this key act on live accounts or remove data. Grant them only when you need them.</Notice>
      ) : null}
    </div>
  );
}
