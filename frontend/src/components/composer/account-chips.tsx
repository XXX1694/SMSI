import { Check } from 'lucide-react';
import { providerLabel } from '@/lib/normalize';
import type { SocialAccount } from '@/lib/types';
import { cn } from '@/lib/utils';

export function AccountChips({
  accounts,
  selected,
  onToggle,
}: {
  accounts: SocialAccount[];
  selected: string[];
  onToggle: (id: string) => void;
}) {
  return (
    <div role="group" aria-label="Publish to" className="flex flex-wrap gap-2">
      {accounts.map((a) => {
        const on = selected.includes(a.id);
        const usable = a.status === 'active';
        return (
          <button
            key={a.id}
            type="button"
            aria-pressed={on}
            disabled={!usable}
            onClick={() => onToggle(a.id)}
            title={usable ? undefined : `Connection is ${a.status}`}
            className={cn(
              'inline-flex items-center gap-1.5 rounded-full border px-3 py-1 text-sm disabled:cursor-not-allowed disabled:opacity-50',
              on ? 'border-accent bg-accent-soft text-accent' : 'hover:bg-muted',
            )}
          >
            {on ? <Check className="h-3.5 w-3.5" aria-hidden /> : null}
            <span className="font-medium">{a.display_name || a.username}</span>
            <span className="text-xs text-muted-foreground">{providerLabel(a.provider)}</span>
          </button>
        );
      })}
    </div>
  );
}
