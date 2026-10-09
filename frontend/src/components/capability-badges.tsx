import { Badge } from '@/components/ui/badge';
import type { Capabilities } from '@/lib/types';

export function CapabilityBadges({ caps }: { caps: Capabilities }) {
  const items: [string, boolean][] = [
    ['Text', caps.canPublishText],
    ['Image', caps.canPublishImage],
    ['Video', caps.canPublishVideo],
    ['Native scheduling', caps.canSchedule],
    ['Delete', caps.canDelete],
    ['Analytics', caps.canAnalytics],
  ];
  return (
    <ul className="flex flex-wrap gap-1.5" aria-label="Capabilities">
      {items.map(([label, on]) => (
        <li key={label}>
          <Badge tone={on ? 'neutral' : 'outline'} className={on ? '' : 'line-through'}>
            <span className="sr-only">{on ? 'Supports ' : 'No '}</span>
            {label}
          </Badge>
        </li>
      ))}
      {caps.maxTextLength > 0 ? (
        <li>
          <Badge tone="outline">{caps.maxTextLength.toLocaleString()} chars</Badge>
        </li>
      ) : null}
    </ul>
  );
}
