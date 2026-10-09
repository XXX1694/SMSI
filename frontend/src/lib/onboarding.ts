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
  title: string;
  hint: string;
  href: string;
  action: string;
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
      title: 'Connect a network',
      hint: 'Steerpost publishes to the accounts you connect. Pick LinkedIn, Telegram or another network.',
      href: '/accounts',
      action: 'Connect account',
      done: f.connectedAccounts > 0,
      optional: false,
    },
    {
      id: 'post',
      title: 'Write your first post',
      hint: 'Save a draft or schedule it. Nothing is published until you say so.',
      href: '/compose',
      action: 'Write a post',
      done: f.hasPost,
      optional: false,
    },
    {
      id: 'agent',
      title: 'Connect an AI agent',
      hint: 'Create an MCP connection or an API key so Claude or your own scripts can draft and schedule for you.',
      href: '/developer/mcp',
      action: 'Connect agent',
      done: live(f.mcpConnections) || live(f.apiKeys),
      optional: false,
    },
    {
      id: 'approval',
      title: 'Review an approval',
      hint: 'When an agent wants to publish or delete, it waits here for your yes. Optional until an agent asks.',
      href: '/approvals',
      action: 'Open approvals',
      done: f.hasApproval,
      optional: true,
    },
  ];
}

export const requiredDone = (steps: OnboardingStep[]): boolean => steps.filter((s) => !s.optional).every((s) => s.done);
