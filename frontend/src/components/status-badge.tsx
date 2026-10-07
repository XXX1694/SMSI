import { Badge } from '@/components/ui/badge';
import { accountStatusView, attemptStatusView, postStatusView, targetStatusView, type StatusView } from '@/lib/status';

function render(v: StatusView) {
  return <Badge tone={v.tone}>{v.label}</Badge>;
}
export const PostStatusBadge = ({ status }: { status: string }) => render(postStatusView(status));
export const TargetStatusBadge = ({ status }: { status: string }) => render(targetStatusView(status));
export const AccountStatusBadge = ({ status }: { status: string }) => render(accountStatusView(status));
export const AttemptStatusBadge = ({ status }: { status: string }) => render(attemptStatusView(status));
