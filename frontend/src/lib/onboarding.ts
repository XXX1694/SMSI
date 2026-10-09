import type { ApiKey, McpConnection } from '@/lib/types';

/** The state the "Get started" checklist is derived from. Nothing here is stored: it all comes from the API. */
export interface OnboardingFacts {
  connectedAccounts: number;
  /** Any post at all (draft, scheduled or published). */
  hasPost: boolean;
  apiKeys: Pick<ApiKey, 'revoked_at' | 'expires_at'>[];
  mcpConnections: Pick<McpConnection, 'revoked_at'>[];
  /** Any approval request, pending or decided. */
  hasApproval: boolean;
}

export interface OnboardingStep {
  id: 'network' | 'post' | 'agent' | 'approval';
  href: string;
  done: boolean;
  optional: boolean;
}

export const ONBOARDING_DISMISSED_KEY = 'socialos_onboarding_dismissed';
/** Set once every required step was seen done, so the dashboard stops asking the API. */
export const ONBOARDING_COMPLETE_KEY = 'socialos_onboarding_complete';

export function onboardingSteps(f: OnboardingFacts, now: Date = new Date()): OnboardingStep[] {
  const live = <T extends { revoked_at: string | null; expires_at?: string | null }>(xs: T[]) =>
    xs.some((x) => !x.revoked_at && !(x.expires_at && Date.parse(x.expires_at) <= now.getTime()));
  return [
    {
      id: 'network',
      href: '/accounts',
      done: f.connectedAccounts > 0,
      optional: false,
    },
    {
      id: 'post',
      href: '/compose',
      done: f.hasPost,
      optional: false,
    },
    {
      id: 'agent',
      href: '/developer/mcp',
      done: live(f.mcpConnections) || live(f.apiKeys),
      optional: false,
    },
    {
      id: 'approval',
      href: '/approvals',
      done: f.hasApproval,
      optional: true,
    },
  ];
}

export const requiredDone = (steps: OnboardingStep[]): boolean => steps.filter((s) => !s.optional).every((s) => s.done);
