import { formatDateTime } from './time';
import type { Approval, ApprovalAction } from './types';

const LABELS: Record<ApprovalAction, string> = {
  'post.publish': 'Publish now',
  'post.retry_now': 'Retry now',
  'post.delete': 'Delete post',
  'post.schedule_soon': 'Schedule within minutes',
  'social_account.disconnect': 'Disconnect account',
  'social_account.connect_token': 'Connect with a token',
};

/** Plain-English name of what the agent wants to do; unknown actions (a newer server) show as sent. */
export function actionLabel(action: string): string {
  return LABELS[action as ApprovalAction] ?? action;
}

/** Actions that cannot be taken back or put content on the live networks. */
export function isIrreversible(action: string): boolean {
  return action === 'post.delete' || action === 'social_account.disconnect';
}

export function isOpen(a: Approval, now: Date = new Date()): boolean {
  return a.status === 'pending' && new Date(a.expires_at).getTime() > now.getTime();
}

/** "9 min left", "Under a minute left", or "Expired". */
export function timeLeft(expiresAt: string, now: Date = new Date()): string {
  const ms = new Date(expiresAt).getTime() - now.getTime();
  if (Number.isNaN(ms) || ms <= 0) return 'Expired';
  const min = Math.ceil(ms / 60_000);
  if (ms < 60_000) return 'Under a minute left';
  return min === 1 ? '1 min left' : `${min} min left`;
}

export interface SummaryLine {
  label: string;
  value: string;
}

const KNOWN: [key: string, label: string][] = [
  ['title', 'Title'],
  ['content', 'Text'],
  ['platforms', 'Networks'],
  ['accounts', 'Accounts'],
  ['provider', 'Network'],
  ['username', 'Account'],
  ['scheduled_at', 'Scheduled for'],
];

/** `instance_url` -> `Instance url`. */
function sentence(key: string): string {
  const words = key.replace(/_/g, ' ');
  return words.charAt(0).toUpperCase() + words.slice(1);
}

function asText(v: unknown): string {
  if (Array.isArray(v)) return v.map(String).join(', ');
  return typeof v === 'string' || typeof v === 'number' ? String(v) : '';
}

/** What the owner needs to see to decide: the server's summary, known fields first, in readable form. */
export function summaryLines(a: Approval, timezone: string): SummaryLine[] {
  const out: SummaryLine[] = [];
  const seen = new Set<string>();
  const add = (key: string, label: string) => {
    seen.add(key);
    const raw = a.summary[key];
    const value = key === 'scheduled_at' && typeof raw === 'string' ? formatDateTime(raw, timezone) : asText(raw);
    if (value) out.push({ label, value });
  };
  for (const [key, label] of KNOWN) add(key, label);
  for (const key of Object.keys(a.summary)) if (!seen.has(key)) add(key, sentence(key));
  return out;
}
