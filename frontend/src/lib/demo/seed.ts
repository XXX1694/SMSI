import type {
  ApiKey,
  AuditLog,
  McpConnection,
  Media,
  PostStatus,
  PublicationAttempt,
  SocialAccount,
} from '../types';
import { svgThumb } from './art';
import { seedApprovals } from './approvals';
import { SEED_ID, seedId, seedPostId } from './ids';
import type { DemoPost, DemoState } from './model';

export const DEMO_USER_EMAIL = 'demo@socialos.dev';
export const DEMO_USER_PASSWORD = 'demo12345';

const MIN = 60_000;
const HOUR = 3_600_000;
const DAY = 86_400_000;

type Net = 'linkedin' | 'telegram' | 'mock';

interface Failure {
  code: string;
  message: string;
  attempts: number;
}

interface PostSpec {
  title: string;
  status: PostStatus;
  nets: Net[];
  text: string;
  /** Shorter variant for Telegram when the post was tailored per platform. */
  telegram?: string;
  at: number;
  by?: 'mcp';
  media?: number[];
  failure?: Failure;
  /** For partially published posts: the network that failed. */
  failed?: Net;
}

/** Local wall-clock time `days` from today (the browser's timezone, like the app's default). */
function localAt(base: Date, days: number, hour: number, minute = 0): number {
  return new Date(base.getFullYear(), base.getMonth(), base.getDate() + days, hour, minute).getTime();
}

/** Local midnight on the first day of the month containing `base`. */
function startOfMonth(base: Date): number {
  return new Date(base.getFullYear(), base.getMonth(), 1).getTime();
}

/** Local wall-clock time on a given day of next month. */
function nextMonthAt(base: Date, day: number, hour: number, minute = 0): number {
  return new Date(base.getFullYear(), base.getMonth() + 1, day, hour, minute).getTime();
}

function specs(now: Date): PostSpec[] {
  const t = now.getTime();
  return [
    // 0-8 published
    {
      title: 'Release 2.4 is live',
      status: 'published',
      nets: ['linkedin', 'telegram'],
      text: 'Release 2.4 is live. Dark mode for the mobile app, faster sync on slow connections, and a fix for the notification bug many of you reported. Full notes are in the changelog.',
      telegram: 'Release 2.4 is live: dark mode, faster sync, and the notification fix. Changelog is pinned above.',
      at: localAt(now, -1, 10),
      media: [0],
    },
    {
      title: 'Behind the scenes: how we ship',
      status: 'published',
      nets: ['linkedin'],
      text: 'We ship every Tuesday. A small release train, a checklist that fits on one screen, and a rollback we have actually practised. Here is what we changed this quarter to make Tuesdays boring.',
      at: localAt(now, -3, 9, 30),
    },
    {
      title: 'Weekly tip: keyboard shortcuts',
      status: 'published',
      nets: ['telegram'],
      text: 'Tip of the week: press N anywhere to start a new draft, and Cmd+Enter to save it. Small things, fewer clicks.',
      at: localAt(now, -4, 8),
    },
    {
      title: 'New office, same team',
      status: 'published',
      nets: ['linkedin', 'telegram'],
      text: 'We moved into a new space this week. Same ten people, more whiteboards, and a kitchen that finally has a working kettle.',
      at: localAt(now, -6, 12, 15),
      media: [1],
    },
    {
      title: 'Customer story: Harbor Labs',
      status: 'published',
      nets: ['linkedin'],
      text: 'Harbor Labs cut the time they spend on weekly updates from three hours to forty minutes. We sat down with their team to talk about what changed.',
      at: localAt(now, -8, 11),
      by: 'mcp',
    },
    {
      title: 'Changelog digest',
      status: 'published',
      nets: ['telegram', 'mock'],
      text: 'This fortnight: CSV export, a calmer settings page, and 31 small fixes. Details in the changelog.',
      at: localAt(now, -10, 15),
    },
    {
      title: 'We are hiring a backend engineer',
      status: 'published',
      nets: ['linkedin'],
      text: 'We are looking for a backend engineer who likes boring, reliable systems. Go, Postgres, a queue, and a team that reads each other\'s pull requests.',
      at: localAt(now, -13, 9),
    },
    {
      title: 'Roadmap preview',
      status: 'published',
      nets: ['linkedin', 'telegram'],
      text: 'A look at what is next: better scheduling, per-platform previews and a public changelog. Tell us what you would move up the list.',
      at: localAt(now, -17, 10, 30),
      media: [2],
    },
    {
      title: 'Status update: sync incident resolved',
      status: 'published',
      nets: ['telegram'],
      text: 'The sync delay reported earlier today is resolved. Everything queued during the incident has been delivered. A short write-up will follow.',
      // Never before the start of the current month, so "published this month" is never empty.
      at: Math.max(t - 25 * MIN, startOfMonth(now)),
    },
    // 9 partially published, 10-11 failed
    {
      title: 'Launch week recap',
      status: 'partially_published',
      nets: ['linkedin', 'telegram'],
      text: 'Launch week in numbers: 1,200 sign-ups, 38 customer calls and exactly one memorable demo failure. Thank you to everyone who tried it early.',
      at: localAt(now, -5, 16),
      failed: 'linkedin',
      failure: { code: 'PROVIDER_ERROR', message: 'LinkedIn returned 502 Bad Gateway after 5 attempts', attempts: 3 },
    },
    {
      title: 'Webinar reminder',
      status: 'failed',
      nets: ['telegram'],
      text: 'Reminder: our webinar on calmer release processes starts in one hour. Link in the pinned message.',
      at: localAt(now, -2, 13),
      failure: { code: 'PROVIDER_ERROR', message: 'Telegram: Bad Request: chat not found', attempts: 2 },
    },
    {
      title: 'Poll: what should we build next?',
      status: 'failed',
      nets: ['linkedin'],
      text: 'Quick poll for the people who follow along: what should we build next, bulk editing or a public API for reports?',
      at: localAt(now, -9, 14),
      failure: { code: 'SOCIAL_ACCOUNT_EXPIRED', message: 'LinkedIn authorization has expired', attempts: 2 },
    },
    // 12-19 scheduled
    {
      title: 'Release 2.5 teaser',
      status: 'scheduled',
      nets: ['linkedin', 'telegram'],
      text: 'Something small but long requested lands next week. A hint: it has to do with Mondays.',
      at: localAt(now, 1, 9),
    },
    {
      title: 'Weekly tip: bulk scheduling',
      status: 'scheduled',
      nets: ['telegram'],
      text: 'Tip of the week: select several drafts on the Posts page and schedule them in one go. Plan the week in ten minutes.',
      at: localAt(now, 2, 8),
    },
    {
      title: 'Podcast: shipping calmly',
      status: 'scheduled',
      nets: ['linkedin'],
      text: 'New episode: we talk about release trains, feature flags and why deploying on Fridays is fine if you are prepared.',
      at: localAt(now, 4, 12),
    },
    {
      title: 'Office hours',
      status: 'scheduled',
      nets: ['telegram', 'mock'],
      text: 'Open office hours on Thursday, 16:00. Bring questions, bugs or strong opinions.',
      at: localAt(now, 6, 16),
    },
    {
      title: 'Year in review',
      status: 'scheduled',
      nets: ['linkedin', 'telegram'],
      text: 'The year in review: what shipped, what we dropped and what we learned about saying no. Thank you for the feedback that shaped it.',
      at: nextMonthAt(now, 3, 10),
      by: 'mcp',
    },
    {
      title: 'Customer story: Greenfield Co',
      status: 'scheduled',
      nets: ['linkedin'],
      text: 'Greenfield Co manages eleven brands from one calendar. Their marketing lead walks through the setup.',
      at: nextMonthAt(now, 9, 9, 30),
    },
    {
      title: 'Conference talk announcement',
      status: 'scheduled',
      nets: ['linkedin', 'telegram'],
      text: 'We are speaking next month about agents that publish with guardrails. Come say hello after the talk.',
      at: nextMonthAt(now, 14, 11),
      media: [3],
    },
    {
      title: 'Weekly tip: previews',
      status: 'scheduled',
      nets: ['telegram'],
      text: 'Tip of the week: the preview panel shows how a post looks on each network before you publish.',
      at: nextMonthAt(now, 21, 8),
    },
    // 20-22 drafts, 23 cancelled
    {
      title: 'Case study draft',
      status: 'draft',
      nets: ['linkedin'],
      text: 'Case study draft: how a three-person team halved their publishing time. Needs numbers from the customer.',
      at: t - 2 * DAY,
    },
    {
      title: 'Launch checklist idea',
      status: 'draft',
      nets: ['telegram'],
      text: 'A launch checklist that fits on one screen: copy, images, links, timezone, second pair of eyes.',
      at: t - 5 * HOUR,
      by: 'mcp',
    },
    {
      title: 'Security update',
      status: 'draft',
      nets: ['linkedin', 'telegram'],
      text: 'Security update: we rotated all signing keys and shortened session lifetimes. Nothing for you to do.',
      at: t - 1 * DAY,
    },
    {
      title: 'Old announcement',
      status: 'cancelled',
      nets: ['linkedin'],
      text: 'Announcement postponed until the details are final.',
      at: localAt(now, -1, 15),
    },
  ];
}

export const SEEDED_POST_COUNT = 24;

const hex = (n: number, width = 8): string => n.toString(16).padStart(width, '0').slice(-width);

export function buildSeed(now: Date = new Date()): DemoState {
  const t = now.getTime();
  const iso = (ms: number): string => new Date(ms).toISOString();
  const pastIso = (hours: number): string => iso(t - hours * HOUR);

  const accounts: SocialAccount[] = [
    {
      id: SEED_ID.account.linkedin,
      provider: 'linkedin',
      username: 'jordan-lee',
      display_name: 'Jordan Lee',
      avatar_url: null,
      status: 'active',
      scopes: ['openid', 'profile', 'email', 'w_member_social'],
      connected_at: pastIso(24 * 41),
    },
    {
      id: SEED_ID.account.telegram,
      provider: 'telegram',
      username: '@studio_updates',
      display_name: 'Studio Updates',
      avatar_url: null,
      status: 'active',
      scopes: ['post_messages'],
      connected_at: pastIso(24 * 39),
    },
    {
      id: SEED_ID.account.mock,
      provider: 'mock',
      username: 'mock-demo',
      display_name: 'Mock Network',
      avatar_url: null,
      status: 'active',
      scopes: [],
      connected_at: pastIso(24 * 30),
    },
  ];
  const account = (n: Net): SocialAccount => {
    const a = accounts.find((x) => x.provider === n);
    if (!a) throw new Error(`seed account ${n}`);
    return a;
  };

  const media: Media[] = [
    { name: 'release-2-4-banner.png', hue: 232, kind: 'image' as const },
    { name: 'new-office.png', hue: 24, kind: 'image' as const },
    { name: 'roadmap-preview.png', hue: 160, kind: 'image' as const },
    { name: 'talk-walkthrough.mp4', hue: 0, kind: 'video' as const },
  ].map((m, i) => ({
    id: SEED_ID.media[i] ?? seedId('b0000000', i + 1),
    kind: m.kind,
    mime_type: m.kind === 'image' ? 'image/png' : 'video/mp4',
    size_bytes: m.kind === 'image' ? 182_000 + i * 41_000 : 8_400_000,
    original_name: m.name,
    width: m.kind === 'image' ? 400 : null,
    height: m.kind === 'image' ? 400 : null,
    status: 'ready',
    url: m.kind === 'image' ? svgThumb(m.name.replace(/\.png$/, ''), m.hue) : undefined,
    created_at: pastIso(24 * (20 - i * 3)),
  }));

  const audit: AuditLog[] = [];
  let auditN = 0;
  const log = (
    at: number,
    actor_type: AuditLog['actor_type'],
    actor_label: string,
    action: string,
    resource_type: string,
    resource_id: string | null,
    metadata?: Record<string, unknown>,
  ): void => {
    auditN += 1;
    audit.push({
      id: seedId('9a000000', auditN),
      actor_type,
      actor_label,
      action,
      resource_type,
      resource_id,
      request_id: hex(auditN * 2654435761 >>> 0),
      ip: actor_type === 'user' ? '203.0.113.24' : actor_type === 'api_key' ? '198.51.100.7' : null,
      created_at: iso(at),
      ...(metadata ? { metadata } : {}),
    });
  };

  const posts: DemoPost[] = specs(now).map((s, i): DemoPost => {
    const id = seedPostId(i);
    const created = Math.min(s.at - 6 * HOUR, t - 30 * MIN);
    const byMcp = s.by === 'mcp';
    const actor = byMcp ? 'Claude Desktop' : 'Demo User';
    const actorType: AuditLog['actor_type'] = byMcp ? 'api_key' : 'user';
    const content = (n: Net): string => (n === 'telegram' && s.telegram ? s.telegram : s.text);

    const targets = s.nets.map((n, ti) => {
      const acc = account(n);
      const targetId = seedId('f1000000', i * 10 + ti + 1);
      const published = s.status === 'published' || (s.status === 'partially_published' && s.failed !== n);
      const failed = s.status === 'failed' || (s.status === 'partially_published' && s.failed === n);
      const when = Math.min(s.at + 2000 * (ti + 1), t);
      const status = published
        ? ('published' as const)
        : failed
          ? ('failed' as const)
          : s.status === 'cancelled'
            ? ('cancelled' as const)
            : ('pending' as const);
      return {
        id: targetId,
        social_account_id: acc.id,
        platform: n,
        content: content(n),
        status,
        external_url: published ? `https://example.com/${n}/${hex(i * 977 + ti * 31 + 7, 6)}` : null,
        published_at: published ? iso(when) : null,
        error_code: failed ? (s.failure?.code ?? 'PROVIDER_ERROR') : null,
        error_message: failed ? (s.failure?.message ?? 'Provider rejected the request') : null,
        attempt_count: published ? 1 : failed ? (s.failure?.attempts ?? 1) : 0,
      };
    });

    const attempts: PublicationAttempt[] = [];
    targets.forEach((tg, ti) => {
      const startedBase = s.at + ti * 3000;
      if (tg.status === 'published') {
        attempts.push({
          id: seedId('f2000000', i * 10 + ti + 1),
          post_target_id: tg.id,
          attempt_no: 1,
          status: 'succeeded',
          started_at: iso(startedBase),
          finished_at: iso(startedBase + 1800),
          error_code: null,
          error_message: null,
        });
      } else if (tg.status === 'failed') {
        for (let n = 1; n <= tg.attempt_count; n += 1) {
          attempts.push({
            id: seedId('f2000000', i * 10 + ti * 4 + n + 500),
            post_target_id: tg.id,
            attempt_no: n,
            status: 'failed',
            started_at: iso(startedBase + (n - 1) * 90_000),
            finished_at: iso(startedBase + (n - 1) * 90_000 + 1200),
            error_code: tg.error_code,
            error_message: tg.error_message,
          });
        }
      }
    });

    const publishedAt = targets
      .map((x) => x.published_at)
      .filter((x): x is string => x !== null)
      .sort()
      .pop();
    const isDraft = s.status === 'draft';

    // Audit trail for this post.
    log(created, actorType, actor, 'post.created', 'post', id);
    if (!isDraft) log(created + 90_000, actorType, actor, 'post.scheduled', 'post', id);
    if (s.status === 'cancelled') log(s.at - 3 * HOUR, 'user', 'Demo User', 'post.cancelled', 'post', id);
    for (const tg of targets) {
      if (tg.status === 'published' && tg.published_at) {
        log(Date.parse(tg.published_at), 'scheduler', 'scheduler', 'post_target.published', 'post_target', tg.id);
      } else if (tg.status === 'failed') {
        log(s.at + 120_000, 'scheduler', 'scheduler', 'post_target.failed', 'post_target', tg.id);
      }
    }

    return {
      id,
      title: s.title,
      content: targets[0]?.content ?? s.text,
      status: s.status,
      scheduled_at: isDraft ? null : iso(s.at),
      published_at: publishedAt ?? null,
      created_by: byMcp ? 'api_key' : 'user',
      created_by_ref: byMcp ? 'Claude Desktop' : null,
      created_at: iso(created),
      updated_at: iso(Math.max(created, publishedAt ? Date.parse(publishedAt) : created)),
      targets,
      media: undefined,
      attempts,
      media_ids: (s.media ?? []).map((m) => SEED_ID.media[m] ?? ''),
      settle_at: null,
    };
  });

  const api_keys: ApiKey[] = [
    {
      id: SEED_ID.apiKey[0],
      name: 'CI reader',
      prefix: 'sk_live_7fQ2',
      scopes: ['social:read', 'posts:read'],
      expires_at: iso(t + 180 * DAY),
      revoked_at: null,
      last_used_at: pastIso(2),
      created_at: pastIso(24 * 100),
    },
    {
      id: SEED_ID.apiKey[1],
      name: 'Release bot',
      prefix: 'sk_live_Kd91',
      scopes: ['social:read', 'posts:read', 'posts:write', 'posts:schedule'],
      expires_at: null,
      revoked_at: null,
      last_used_at: pastIso(26),
      created_at: pastIso(24 * 45),
    },
    {
      id: SEED_ID.apiKey[2],
      name: 'Old laptop',
      prefix: 'sk_live_x8Wm',
      scopes: ['social:read', 'posts:read'],
      expires_at: null,
      revoked_at: pastIso(24 * 20),
      last_used_at: pastIso(24 * 21),
      created_at: pastIso(24 * 120),
    },
  ];

  const mcp_connections: McpConnection[] = [
    {
      id: SEED_ID.mcp[0],
      name: 'Claude Desktop',
      client_name: 'claude-desktop 1.4',
      scopes: ['social:read', 'posts:read', 'analytics:read', 'posts:write', 'media:write', 'posts:schedule'],
      last_seen_at: pastIso(3),
      revoked_at: null,
      created_at: pastIso(24 * 50),
    },
    {
      id: SEED_ID.mcp[1],
      name: 'Cursor',
      client_name: 'cursor 0.45',
      scopes: ['social:read', 'posts:read', 'analytics:read'],
      last_seen_at: pastIso(50),
      revoked_at: null,
      created_at: pastIso(24 * 12),
    },
    {
      id: SEED_ID.mcp[2],
      name: 'n8n workflow',
      client_name: null,
      scopes: ['social:read', 'posts:read', 'posts:write'],
      last_seen_at: pastIso(24 * 30),
      revoked_at: pastIso(24 * 18),
      created_at: pastIso(24 * 60),
    },
  ];

  // Account, key and login events.
  log(t - 41 * DAY, 'user', 'Demo User', 'social_account.connected', 'social_account', SEED_ID.account.linkedin);
  log(t - 39 * DAY, 'system', 'telegram', 'social_account.connected', 'social_account', SEED_ID.account.telegram);
  log(t - 30 * DAY, 'user', 'Demo User', 'social_account.connected', 'social_account', SEED_ID.account.mock);
  log(t - 100 * DAY, 'user', 'Demo User', 'api_key.created', 'api_key', SEED_ID.apiKey[0]);
  log(t - 50 * DAY, 'user', 'Demo User', 'mcp_connection.created', 'mcp_connection', SEED_ID.mcp[0]);
  log(t - 20 * DAY, 'user', 'Demo User', 'api_key.revoked', 'api_key', SEED_ID.apiKey[2]);
  log(t - 18 * DAY, 'user', 'Demo User', 'mcp_connection.revoked', 'mcp_connection', SEED_ID.mcp[2]);
  log(t - 3 * HOUR, 'api_key', 'Claude Desktop', 'api_key.request', 'post', null);
  log(t - 2 * HOUR, 'api_key', 'CI reader', 'api_key.request', 'post', null);
  // Tool calls made by MCP agents (what the backend records for X-MCP-Tool requests).
  const toolCall = (at: number, client: string, tool: string, route: string, method: string, status: number, errorCode?: string, direct = false): void =>
    log(at, 'api_key', client, 'mcp.tool_call', 'api_key', SEED_ID.apiKey[0], {
      tool,
      via_gateway: !direct,
      method,
      route,
      status,
      client,
      ...(errorCode ? { error_code: errorCode } : {}),
    });
  toolCall(t - 3 * HOUR - 5 * MIN, 'MCP: Claude Desktop', 'list_posts', '/api/v1/posts', 'GET', 200);
  toolCall(t - 3 * HOUR - 2 * MIN, 'MCP: Claude Desktop', 'create_draft', '/api/v1/posts', 'POST', 201);
  toolCall(t - 90 * MIN, 'MCP: Claude Desktop', 'publish_post', '/api/v1/posts/{id}/publish', 'POST', 403, 'INSUFFICIENT_SCOPE');
  toolCall(t - 55 * MIN, 'MCP: Cursor', 'get_post_status', '/api/v1/posts/{id}/status', 'GET', 200);
  toolCall(t - 70 * MIN, 'MCP: Script', 'list_posts', '/api/v1/posts', 'GET', 200, undefined, true);
  log(t - 40 * MIN, 'user', 'Demo User', 'user.login', 'user', SEED_ID.user);
  audit.sort((a, b) => b.created_at.localeCompare(a.created_at) || b.id.localeCompare(a.id));

  return {
    version: 1,
    seeded_at: iso(t),
    signed_in: true,
    user: { id: SEED_ID.user, email: DEMO_USER_EMAIL, password: DEMO_USER_PASSWORD, display_name: 'Demo User' },
    accounts,
    posts,
    media,
    api_keys,
    mcp_connections,
    audit,
    approvals: seedApprovals(t),
    links: [],
    usage: {
      [SEED_ID.apiKey[0]]: 412,
      [SEED_ID.apiKey[1]]: 128,
      [SEED_ID.mcp[0]]: 236,
      [SEED_ID.mcp[1]]: 41,
    },
  };
}
