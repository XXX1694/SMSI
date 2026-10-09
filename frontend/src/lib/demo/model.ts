import type {
  AnalyticsPoint,
  ApiKey,
  Approval,
  AuditLog,
  McpConnection,
  Media,
  Post,
  SocialAccount,
} from '../types';

/** The shapes below are the real API types (`../types`); only server-side bookkeeping is added. */

export interface DemoUser {
  id: string;
  email: string;
  password: string;
  display_name: string;
}

/** A post as stored: the public `Post` plus fields the server keeps but never returns. */
export interface DemoPost extends Post {
  media_ids: string[];
  /** While `publishing`: when the simulated provider call finishes. */
  settle_at: string | null;
  /** Set once the post was scheduled or published: it then counts against the monthly limit, like `quota_counted_at` in the API. */
  quota_counted?: boolean;
}

/** A pending/connected Telegram "post this code in your chat" request. */
export interface DemoLink {
  id: string;
  code: string;
  created_at: string;
  expires_at: string;
  account_id: string | null;
}

export interface DemoState {
  version: 1;
  seeded_at: string;
  /** False after "Sign out"; the demo signs in again with the pre-filled credentials. */
  signed_in: boolean;
  user: DemoUser;
  accounts: SocialAccount[];
  posts: DemoPost[];
  media: Media[];
  api_keys: ApiKey[];
  mcp_connections: McpConnection[];
  audit: AuditLog[];
  approvals: Approval[];
  links: DemoLink[];
  /** Request counts per key / connection id, shown on the usage panel. */
  usage: Record<string, number>;
}

export type Method = 'GET' | 'POST' | 'PATCH' | 'DELETE';

export interface DemoRequest {
  method: Method;
  /** Path below `/api/v1`, e.g. `/posts/123/schedule`. */
  path: string;
  query?: Record<string, string | number | undefined | null>;
  body?: unknown;
  /** A file the user picked for `POST /media`, already described by the browser glue. */
  upload?: { name: string; mime: string; size: number; url?: string };
}

export interface DemoResponse {
  status: number;
  /** Parsed JSON body (`undefined` for 204). Errors use the uniform `{error:{code,message,request_id}}`. */
  body: unknown;
}

export interface WireConnectField {
  name: string;
  label: string;
  help?: string;
  placeholder?: string;
  kind: 'text' | 'secret' | 'url';
  required: boolean;
  secret?: boolean;
}

/** Raw provider as `GET /social/providers` returns it (snake_case, normalised by the client). */
export interface WireProvider {
  provider: string;
  configured: boolean;
  status: 'supported' | 'unsupported';
  capabilities: {
    can_publish_text: boolean;
    can_publish_image: boolean;
    can_publish_video: boolean;
    can_schedule: boolean;
    can_delete: boolean;
    can_analytics: boolean;
    max_text_length: number;
    max_media_count: number;
    requires_approval: boolean;
    notes: string;
    connect_method?: string;
    connect_fields?: WireConnectField[];
  };
}

export type { AnalyticsPoint };
