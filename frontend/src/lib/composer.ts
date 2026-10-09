import type { AppT } from '@/i18n/translate';
import type { Media, Provider, SocialAccount } from './types';

export interface ComposerState {
  content: string;
  /** Per-account content overrides keyed by account id. Empty string = no override. */
  overrides: Record<string, string>;
  accountIds: string[];
  media: Media[];
  /** UTC ISO string for scheduled mode, or null. */
  scheduledAtUtc: string | null;
}

export interface ValidationIssue {
  /** Account id this applies to, or null for global. */
  accountId: string | null;
  message: string;
}

export function effectiveContent(state: Pick<ComposerState, 'content' | 'overrides'>, accountId: string): string {
  const o = state.overrides[accountId];
  return o !== undefined && o.trim() !== '' ? o : state.content;
}

/** Character count as the user sees it (code points, not UTF-16 units). */
export function charCount(text: string): number {
  return Array.from(text).length;
}

export function validateComposer(
  state: ComposerState,
  accounts: SocialAccount[],
  providers: Provider[],
  t: AppT,
  opts: { requireSchedule?: boolean; now?: Date } = {},
): ValidationIssue[] {
  const issues: ValidationIssue[] = [];
  if (state.accountIds.length === 0) {
    issues.push({ accountId: null, message: t('composer.v.selectAccount') });
  }
  const hasMedia = state.media.length > 0;
  for (const id of state.accountIds) {
    const account = accounts.find((a) => a.id === id);
    if (!account) {
      issues.push({ accountId: id, message: t('composer.v.accountGone') });
      continue;
    }
    const label = account.display_name || account.username;
    if (account.status !== 'active') {
      issues.push({ accountId: id, message: t('composer.v.needsReconnect', { account: label }) });
    }
    const provider = providers.find((p) => p.id === account.provider);
    const text = effectiveContent(state, id);
    if (text.trim() === '' && !hasMedia) {
      issues.push({ accountId: id, message: t('composer.v.addText', { account: label }) });
    }
    if (!provider) continue;
    const caps = provider.capabilities;
    const len = charCount(text);
    if (caps.maxTextLength > 0 && len > caps.maxTextLength) {
      issues.push({
        accountId: id,
        message: t('composer.v.overLimit', { account: label, over: len - caps.maxTextLength, network: provider.name, limit: caps.maxTextLength }),
      });
    }
    if (caps.maxMediaCount >= 0 && state.media.length > caps.maxMediaCount) {
      issues.push({
        accountId: id,
        message: t('composer.v.mediaMax', { account: label, network: provider.name, max: caps.maxMediaCount }),
      });
    }
    if (state.media.some((m) => m.kind === 'image') && !caps.canPublishImage) {
      issues.push({ accountId: id, message: t('composer.v.noImages', { account: label, network: provider.name }) });
    }
    if (state.media.some((m) => m.kind === 'video') && !caps.canPublishVideo) {
      issues.push({ accountId: id, message: t('composer.v.noVideo', { account: label, network: provider.name }) });
    }
  }
  if (opts.requireSchedule) {
    const now = opts.now ?? new Date();
    if (!state.scheduledAtUtc) {
      issues.push({ accountId: null, message: t('composer.v.chooseTime') });
    } else if (new Date(state.scheduledAtUtc).getTime() <= now.getTime() + 60_000) {
      issues.push({ accountId: null, message: t('composer.v.tooSoon') });
    }
  }
  return issues;
}

export function counterTone(len: number, max: number): 'ok' | 'warn' | 'over' {
  if (max <= 0) return 'ok';
  if (len > max) return 'over';
  if (len > max * 0.9) return 'warn';
  return 'ok';
}
