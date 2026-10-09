'use client';
import { Badge } from '@/components/ui/badge';
import { useTranslations } from '@/i18n/use-translations';
import { accountStatusView, attemptStatusView, postStatusView, targetStatusView, type StatusView } from '@/lib/status';
import type { AppT } from '@/i18n/translate';

function render(v: StatusView) {
  return (
    <Badge tone={v.tone} glyph={v.glyph}>
      {v.label}
    </Badge>
  );
}
function badge(view: (s: string, t: AppT) => StatusView) {
  return function StatusBadge({ status }: { status: string }) {
    const t = useTranslations();
    return render(view(status, t));
  };
}
export const PostStatusBadge = badge(postStatusView);
export const TargetStatusBadge = badge(targetStatusView);
export const AccountStatusBadge = badge(accountStatusView);
export const AttemptStatusBadge = badge(attemptStatusView);
