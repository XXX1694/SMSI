import { providerLabel } from './normalize';
import { postStatusView } from './status';
import { formatDateTime } from './time';
import { joinList } from './format';
import type { AppT } from '@/i18n/translate';
import type { Approval, ApprovalAction } from './types';

const LABEL_KEYS = {
  'post.publish': 'approvals.action.post_publish',
  'post.retry_now': 'approvals.action.post_retry_now',
  'post.delete': 'approvals.action.post_delete',
  'post.schedule_soon': 'approvals.action.post_schedule_soon',
  'social_account.disconnect': 'approvals.action.social_account_disconnect',
  'social_account.connect_token': 'approvals.action.social_account_connect_token',
} as const satisfies Record<ApprovalAction, string>;

/** Plain-language name of what the agent wants to do; unknown actions (a newer server) show as sent. */
export function actionLabel(action: string, t: AppT): string {
  return action in LABEL_KEYS ? t(LABEL_KEYS[action as ApprovalAction]) : action;
}

/** Actions that cannot be taken back or put content on the live networks. */
export function isIrreversible(action: string): boolean {
  return action === 'post.delete' || action === 'social_account.disconnect' || action === 'post.publish' || action === 'post.retry_now';
}

export function isOpen(a: Approval, now: Date = new Date()): boolean {
  return a.status === 'pending' && new Date(a.expires_at).getTime() > now.getTime();
}

/** "9 min left", "Under a minute left", or "Expired". */
export function timeLeft(expiresAt: string, t: AppT, now: Date = new Date()): string {
  const ms = new Date(expiresAt).getTime() - now.getTime();
  if (Number.isNaN(ms) || ms <= 0) return t('approvals.status.expired');
  const min = Math.ceil(ms / 60_000);
  if (ms < 60_000) return t('approvals.underMinute');
  return t('approvals.minLeft', { count: min });
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

const KNOWN = [
  ['title', 'approvals.summary.title'],
  ['content', 'approvals.summary.text'],
  ['targets', null],
  ['media', 'approvals.summary.media'],
  ['platforms', 'approvals.summary.networks'],
  ['accounts', 'approvals.summary.accounts'],
  ['provider', 'approvals.summary.network'],
  ['username', 'approvals.summary.account'],
  ['scheduled_at', 'approvals.summary.scheduledFor'],
] as const;

const isRec = (v: unknown): v is Record<string, unknown> => typeof v === 'object' && v !== null && !Array.isArray(v);

/** `instance_url` -> `Instance url`. */
function sentence(key: string): string {
  const words = key.replace(/_/g, ' ');
  return words.charAt(0).toUpperCase() + words.slice(1);
}

/** `linkedin · @alex` -> `LinkedIn · @alex`: the server sends network ids. */
function named(account: string): string {
  return account.replace(/^[a-z0-9_]+(?= · )/, providerLabel);
}

function isLong(value: string): boolean {
  return value.length > CLAMP_CHARS || value.split('\n').length > CLAMP_LINES;
}

function mediaText(m: Record<string, unknown>, t: AppT): string {
  const images = Number(m.images) || 0;
  const videos = Number(m.videos) || 0;
  const count = Number(m.count) || 0;
  const parts = [images ? t('approvals.summary.images', { count: images }) : '', videos ? t('approvals.summary.videos', { count: videos }) : ''].filter(Boolean);
  if (parts.length) return joinList(parts, t);
  return count ? t('approvals.summary.files', { count }) : '';
}

function asText(v: unknown): string {
  if (Array.isArray(v)) return v.map(String).join(', ');
  return typeof v === 'string' || typeof v === 'number' ? String(v) : '';
}

/** What the owner needs to see to decide: the server's summary, known fields first, in readable form. */
export function summaryLines(a: Approval, timezone: string, t: AppT): SummaryLine[] {
  const out: SummaryLine[] = [];
  const add = (label: string, value: string) => {
    if (value) out.push({ label, value, long: isLong(value) });
  };
  for (const [key, labelKey] of KNOWN) {
    const label = labelKey ? t(labelKey) : '';
    const raw = a.summary[key];
    if (key === 'platforms' && Array.isArray(a.summary.accounts)) continue; // the accounts line says it with names
    if (key === 'targets' && Array.isArray(raw)) {
      for (const tg of raw) if (isRec(tg)) add(t('approvals.summary.textOn', { account: named(asText(tg.account)) || providerLabel(asText(tg.platform)) }), asText(tg.content));
    } else if (key === 'media' && isRec(raw)) add(label, mediaText(raw, t));
    else if (key === 'scheduled_at' && typeof raw === 'string') add(label, formatDateTime(raw, timezone, t.locale));
    else if ((key === 'platforms' || key === 'provider') && asText(raw)) add(label, joinList(asText(raw).split(', ').map(providerLabel), t));
    else if (key === 'accounts' && Array.isArray(raw)) add(label, joinList(raw.map((x) => named(String(x))), t));
    else add(label, asText(raw));
  }
  for (const key of Object.keys(a.summary)) {
    if (KNOWN.some(([k]) => k === key)) continue;
    // A post status the server sent as a code ("draft") reads as the badge text ("Draft").
    add(sentence(key), key === 'status' ? postStatusView(asText(a.summary[key]), t).label : asText(a.summary[key]));
  }
  return out;
}
