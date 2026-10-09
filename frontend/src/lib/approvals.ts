import { providerLabel } from './normalize';
import { postStatusView } from './status';
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
  /** Long enough to be clamped in the card; the owner can expand it to read all of it. */
  long: boolean;
}

/** Past this many characters or lines a text is clamped in the card. */
const CLAMP_CHARS = 280;
const CLAMP_LINES = 4;

const KNOWN: [key: string, label: string][] = [
  ['title', 'Title'],
  ['content', 'Text'],
  ['targets', ''],
  ['media', 'Media'],
  ['platforms', 'Networks'],
  ['accounts', 'Accounts'],
  ['provider', 'Network'],
  ['username', 'Account'],
  ['scheduled_at', 'Scheduled for'],
];

const isRec = (v: unknown): v is Record<string, unknown> => typeof v === 'object' && v !== null && !Array.isArray(v);

/** `instance_url` -> `Instance url`. */
function sentence(key: string): string {
  const words = key.replace(/_/g, ' ');
  return words.charAt(0).toUpperCase() + words.slice(1);
}

function isLong(value: string): boolean {
  return value.length > CLAMP_CHARS || value.split('\n').length > CLAMP_LINES;
}

function plural(n: number, one: string): string {
  return `${n} ${one}${n === 1 ? '' : 's'}`;
}

function mediaText(m: Record<string, unknown>): string {
  const images = Number(m.images) || 0;
  const videos = Number(m.videos) || 0;
  const count = Number(m.count) || 0;
  const parts = [images ? plural(images, 'image') : '', videos ? plural(videos, 'video') : ''].filter(Boolean);
  if (parts.length) return parts.join(', ');
  return count ? plural(count, 'file') : '';
}

/** `linkedin · @demo` -> `LinkedIn (@demo)`; a bare id (`telegram`) -> `Telegram`. Anything that does not look like an id stays as sent. */
export function accountText(raw: string): string {
  const [id, ...rest] = raw.split(' · ');
  if (!id || !/^[a-z][a-z0-9_]*$/.test(id)) return raw;
  const name = providerLabel(id);
  const handle = rest.join(' · ').trim();
  return handle ? `${name} (${handle})` : name;
}

function asText(v: unknown): string {
  if (Array.isArray(v)) return v.map(String).join(', ');
  return typeof v === 'string' || typeof v === 'number' ? String(v) : '';
}

/** What the owner needs to see to decide: the server's summary, known fields first, in readable form. */
export function summaryLines(a: Approval, timezone: string): SummaryLine[] {
  const out: SummaryLine[] = [];
  const add = (label: string, value: string) => {
    if (value) out.push({ label, value, long: isLong(value) });
  };
  for (const [key, label] of KNOWN) {
    const raw = a.summary[key];
    if (key === 'platforms' && Array.isArray(a.summary.accounts)) continue; // the accounts line says it with names
    if (key === 'targets' && Array.isArray(raw)) {
      for (const t of raw) if (isRec(t)) add(`Text on ${accountText(asText(t.account) || asText(t.platform))}`, asText(t.content));
    } else if (key === 'media' && isRec(raw)) add(label, mediaText(raw));
    else if (key === 'scheduled_at' && typeof raw === 'string') add(label, formatDateTime(raw, timezone));
    else if (key === 'platforms' || key === 'accounts' || key === 'provider') add(label, asText(raw).split(', ').map(accountText).join(', '));
    else add(label, asText(raw));
  }
  for (const key of Object.keys(a.summary)) {
    if (KNOWN.some(([k]) => k === key)) continue;
    // A post status the server sent as a code ("draft") reads as the badge text ("Draft").
    add(sentence(key), key === 'status' ? postStatusView(asText(a.summary[key])).label : asText(a.summary[key]));
  }
  return out;
}
