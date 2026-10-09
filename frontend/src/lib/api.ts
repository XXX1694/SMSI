import { ApiError, parseErrorBody } from './api-error';
import { API_BASE, buildQuery, enc, request, setCsrfToken, uploadBase } from './api-http';
import {
  buildMcpConfig,
  normalizeCreatedApiKey,
  normalizeCreatedMcp,
  normalizeMe,
  normalizePage,
  normalizeTelegramLink,
  normalizeTelegramLinkState,
  normalizeProvider,
  unwrapList,
} from './normalize';
import type {
  AnalyticsResult,
  ApiKey,
  Approval,
  AuditLog,
  CreatePostInput,
  CreatedApiKey,
  CreatedMcpConnection,
  DashboardSummary,
  DataExport,
  DataExportLink,
  Me,
  McpConnection,
  Media,
  Page,
  Post,
  Provider,
  SocialAccount,
  TelegramLink,
  TelegramLinkState,
  UsageReport,
  UsageSummary,
} from './types';
const MCP_URL = process.env.NEXT_PUBLIC_MCP_URL ?? 'http://localhost:3333/mcp';

export { API_BASE, ApiError, buildQuery, parseErrorBody, setCsrfToken, uploadBase };

export interface PostFilters {
  status?: string;
  from?: string;
  to?: string;
  limit?: number;
  cursor?: string;
}

export const api = {
  auth: {
    async register(input: { email: string; password: string; display_name: string; accept_terms: boolean }): Promise<Me> {
      return normalizeMe(await request('/auth/register', { method: 'POST', body: input }));
    },
    async login(input: { email: string; password: string }): Promise<Me> {
      return normalizeMe(await request('/auth/login', { method: 'POST', body: input }));
    },
    async logout(): Promise<void> {
      await request('/auth/logout', { method: 'POST' });
    },
    async me(): Promise<Me> {
      return normalizeMe(await request('/me'));
    },
    /** Redeems the token from a mailed link. An unknown, used or expired token is a 400. */
    async verifyEmail(token: string): Promise<void> {
      await request('/auth/verify-email', { method: 'POST', body: { token } });
    },
    /** Mails a new verification link to the signed-in user (429 inside the one-minute cooldown). */
    async resendVerification(): Promise<void> {
      await request('/auth/verify-email/resend', { method: 'POST' });
    },
    /** Always succeeds the same way for any address; `delivery` says whether mail can actually leave the server. */
    async forgotPassword(email: string): Promise<{ delivery: 'log' | 'smtp' }> {
      const r = (await request('/auth/password/forgot', { method: 'POST', body: { email } })) as { delivery?: string } | null;
      return { delivery: r?.delivery === 'log' ? 'log' : 'smtp' };
    },
    async resetPassword(token: string, password: string, revokeKeys = false): Promise<void> {
      await request('/auth/password/reset', { method: 'POST', body: { token, password, revoke_keys: revokeKeys } });
    },
    /** Signs every other session out; API keys and MCP connections are revoked only with `revokeKeys`. A wrong current password is a 400. */
    async changePassword(currentPassword: string, newPassword: string, revokeKeys = false): Promise<void> {
      await request('/auth/password/change', {
        method: 'POST',
        body: { current_password: currentPassword, new_password: newPassword, revoke_keys: revokeKeys },
      });
    },
  },
  social: {
    async providers(): Promise<Provider[]> {
      return unwrapList<unknown>(await request('/social/providers')).map(normalizeProvider);
    },
    async accounts(): Promise<SocialAccount[]> {
      return unwrapList<SocialAccount>(await request('/social/accounts'));
    },
    connectUrl(provider: string): string {
      return `${API_BASE}/social/${enc(provider)}/connect?redirect=${enc('/accounts')}`;
    },
    /**
     * Starts proving control of a Telegram channel or group: returns a one-time code to post
     * there (there is no way to connect a chat by name). Session only, CSRF protected.
     */
    async startTelegramLink(): Promise<TelegramLink> {
      return normalizeTelegramLink(await request('/social/telegram/connect', { method: 'POST' }));
    },
    /** Polled while the user posts the code. Another user's link id is a 404. */
    async telegramLinkStatus(id: string): Promise<TelegramLinkState> {
      return normalizeTelegramLinkState(await request(`/social/telegram/connect/${enc(id)}`));
    },
    async disconnect(id: string): Promise<void> {
      await request(`/social/accounts/${enc(id)}`, { method: 'DELETE' });
    },
    /**
     * Connects a network from a pasted credential (Discord webhook, Mastodon token, Bluesky app password).
     * The values go in the request body only: never in a URL, a log or storage. Needs `social:connect`.
     */
    async connectWithToken(provider: string, fields: Record<string, string>): Promise<SocialAccount> {
      return (await request('/social/accounts/token', { method: 'POST', body: { provider, fields } })) as SocialAccount;
    },
    /** Demo build only: OAuth cannot run in a static site, so this adds a sample account right away. */
    async connectDemo(provider: string): Promise<SocialAccount> {
      return (await request(`/social/${enc(provider)}/connect`, { method: 'POST' })) as SocialAccount;
    },
  },
  posts: {
    async list(f: PostFilters = {}): Promise<Page<Post>> {
      return normalizePage<Post>(await request('/posts', { query: { ...f } }));
    },
    async get(id: string): Promise<Post> {
      return (await request(`/posts/${enc(id)}`)) as Post;
    },
    async create(input: CreatePostInput): Promise<Post> {
      return (await request('/posts', { method: 'POST', body: input })) as Post;
    },
    async publish(id: string): Promise<Post> {
      return (await request(`/posts/${enc(id)}/publish`, { method: 'POST' })) as Post;
    },
    async schedule(id: string, scheduledAt: string): Promise<Post> {
      return (await request(`/posts/${enc(id)}/schedule`, { method: 'POST', body: { scheduled_at: scheduledAt } })) as Post;
    },
    async cancel(id: string): Promise<Post> {
      return (await request(`/posts/${enc(id)}/cancel`, { method: 'POST' })) as Post;
    },
    async retry(id: string): Promise<Post> {
      return (await request(`/posts/${enc(id)}/retry`, { method: 'POST' })) as Post;
    },
    async remove(id: string): Promise<void> {
      await request(`/posts/${enc(id)}`, { method: 'DELETE' });
    },
  },
  media: {
    async list(): Promise<Media[]> {
      return unwrapList<Media>(await request('/media'));
    },
    async upload(file: File): Promise<Media> {
      const form = new FormData();
      form.append('file', file);
      return (await request('/media', { method: 'POST', form, direct: true })) as Media;
    },
    async get(id: string): Promise<Media> {
      return (await request(`/media/${enc(id)}`)) as Media;
    },
    async remove(id: string): Promise<void> {
      await request(`/media/${enc(id)}`, { method: 'DELETE' });
    },
  },
  account: {
    async usage(): Promise<UsageReport> {
      return (await request('/account/usage')) as UsageReport;
    },
    /**
     * Schedules deletion of the whole account after the grace period and ends this session (the server clears the
     * cookies). Needs the current password and the account's email typed as confirmation; a wrong one is a 400 with
     * the field named. API keys cannot do this.
     */
    async requestDeletion(password: string, confirm: string): Promise<{ scheduled_for: string }> {
      const r = (await request('/account/delete', { method: 'POST', body: { password, confirm } })) as { scheduled_for?: string } | null;
      return { scheduled_for: r?.scheduled_for ?? '' };
    },
    /** Cancels a scheduled deletion (409 when none is scheduled). */
    async cancelDeletion(): Promise<void> {
      await request('/account/delete/cancel', { method: 'POST' });
    },
    /** Account data export (session only). 409 while one is being prepared, 429 inside the 24 h cooldown. */
    exports: {
      async list(): Promise<DataExport[]> {
        return unwrapList<DataExport>(await request('/account/exports'));
      },
      async request(): Promise<DataExport> {
        return (await request('/account/exports', { method: 'POST' })) as DataExport;
      },
      /** A short-lived download URL for a ready export. */
      async link(id: string): Promise<DataExportLink> {
        return (await request(`/account/exports/${enc(id)}`)) as DataExportLink;
      },
    },
  },
  dashboard: {
    async summary(): Promise<DashboardSummary> {
      const r = (await request('/dashboard/summary')) as Partial<DashboardSummary>;
      return {
        connected_accounts: r.connected_accounts ?? 0,
        scheduled_posts: r.scheduled_posts ?? 0,
        drafts: r.drafts ?? 0,
        published_this_month: r.published_this_month ?? 0,
        failed: r.failed ?? 0,
        upcoming: r.upcoming ?? [],
        recent: r.recent ?? [],
      };
    },
  },
  analytics: {
    async get(from?: string, to?: string): Promise<AnalyticsResult> {
      return { items: unwrapList(await request('/analytics', { query: { from, to } })) };
    },
  },
  audit: {
    /** `action` keeps only entries of that action, e.g. `mcp.tool_call` for agent actions. */
    async list(limit = 50, cursor?: string, action?: string): Promise<Page<AuditLog>> {
      return normalizePage<AuditLog>(await request('/audit-logs', { query: { limit, cursor, action } }));
    },
  },
  approvals: {
    /** `all` includes decided and expired ones; the default is only those waiting for a decision. */
    async list(status: 'pending' | 'all' = 'pending', limit = 25, cursor?: string): Promise<Page<Approval>> {
      return normalizePage<Approval>(await request('/approvals', { query: { status, limit, cursor } }));
    },
    /** The agent may then repeat its call once. A decided or expired approval is a 409. */
    async approve(id: string): Promise<Approval> {
      return (await request(`/approvals/${enc(id)}/approve`, { method: 'POST' })) as Approval;
    },
    async deny(id: string): Promise<Approval> {
      return (await request(`/approvals/${enc(id)}/deny`, { method: 'POST' })) as Approval;
    },
  },
  developer: {
    async apiKeys(): Promise<ApiKey[]> {
      return unwrapList<ApiKey>(await request('/developer/api-keys'));
    },
    async createApiKey(input: { name: string; scopes: string[]; expires_at?: string; dangerous_policy?: 'approve' | 'trusted' }): Promise<CreatedApiKey> {
      return normalizeCreatedApiKey(await request('/developer/api-keys', { method: 'POST', body: input }));
    },
    async revokeApiKey(id: string): Promise<void> {
      await request(`/developer/api-keys/${enc(id)}`, { method: 'DELETE' });
    },
    async mcpConnections(): Promise<McpConnection[]> {
      return unwrapList<McpConnection>(await request('/developer/mcp-connections'));
    },
    async createMcpConnection(input: { name: string; scopes: string[] }): Promise<CreatedMcpConnection> {
      const raw = await request('/developer/mcp-connections', { method: 'POST', body: input });
      return normalizeCreatedMcp(raw, MCP_URL);
    },
    async revokeMcpConnection(id: string): Promise<void> {
      await request(`/developer/mcp-connections/${enc(id)}`, { method: 'DELETE' });
    },
    async usage(): Promise<UsageSummary> {
      const r = (await request('/developer/usage')) as Partial<UsageSummary>;
      return { total_requests: r.total_requests ?? 0, by_key: r.by_key ?? [], by_day: r.by_day ?? [] };
    },
  },
};

export { buildMcpConfig };
