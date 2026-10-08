# SocialOS — Architecture & Contract

This document is the single source of truth for the backend, MCP server and frontend.
All three components are implemented against the contract below.

## 1. Services

```
User (browser) ──► Next.js frontend ─┐
                                      ├──► SocialOS REST API (Go, :8080) ──► Social Core ──► Adapters (LinkedIn, Telegram, Mock…)
AI agent ──► MCP server (TS, :3333) ──┘             │                             │
                                                     ├─ PostgreSQL (source of truth)
                                                     ├─ Redis + Asynq (queue)      ◄── Worker (Go) runs publish jobs
                                                     └─ S3 (MinIO / R2)
```

* **api** – Go HTTP server (chi). Stateless. Auth by session cookie (browser) or `Authorization: Bearer sk_live_…` (API key / MCP).
* **worker** – Go Asynq worker. Same module/images, `cmd/worker`. Runs `post:publish_target` jobs and, in the default `TELEGRAM_UPDATES_MODE=polling`, the single Telegram `getUpdates` long-poll that redeems link codes (§4 "Telegram linking").
* **mcp** – TypeScript, official `@modelcontextprotocol/sdk`, Streamable HTTP transport. Talks only to the REST API using the caller's API key. No DB, no social APIs.
* **frontend** – Next.js (App Router) + Tailwind + shadcn-style components. Talks to the REST API through a same-origin `/api` rewrite so session cookies are first-party.

### Key decisions (MVP: simplest production-ready choice)
| Decision | Choice | Why |
|---|---|---|
| Architecture | Hexagonal: `domain` (pure) → `application` (use cases, ports) → `infrastructure` / `adapters` / `transport` | Core logic has no framework or provider imports |
| HTTP router | chi | Small, idiomatic, std-lib compatible |
| DB access | pgx + hand-written SQL, goose-style SQL migrations | No ORM magic, easy to audit tenant filters |
| Auth (browser) | Opaque server-side sessions in Postgres, HttpOnly SameSite=Lax cookie, CSRF double-submit header for mutating requests | Revocable; simpler than JWT refresh |
| Auth (agents) | API keys `sk_live_…` (SHA-256 hash stored, scopes, expiry, revoke) bound to one user; each MCP connection = one API key | Works with every MCP client; OAuth 2.1 for MCP is a documented post-MVP upgrade |
| Token encryption | AES-256-GCM, key from `ENCRYPTION_KEY` (32 bytes base64), versioned ciphertext prefix `v1:` | Key rotation possible later |
| Queue | Asynq; scheduled jobs use `ProcessAt`; DB `scheduled_jobs` is truth, Redis is transport. A reconciler re-enqueues due-but-missing jobs every minute | Survives Redis flush |
| Idempotency | One `post_target` ⇒ at most one successful publish. Worker takes a row lock (`SELECT … FOR UPDATE SKIP LOCKED`) and an attempt row with unique `(post_target_id, attempt_no)`. Before calling a provider it stores `idempotency_key = post_target.id`. If `external_post_id` already set ⇒ no-op. Adapters that support provider idempotency pass the key; others rely on state check + "attempt in `unknown` state ⇒ adapter.GetPost lookup / mark `needs_review`" | See §6 |
| Storage | S3 API via minio-go; works with MinIO/R2/S3 by endpoint config | |

## 2. Domain rules

### Post state machine (enforced in `domain/post`)
```
draft ──schedule──► scheduled ──job starts──► publishing ──► published | partially_published | failed
  ▲                     │  ▲                                        │
  └────── unschedule ◄──┘  └────────────── retry failed ◄───────────┘ (failed/partially → scheduled|publishing via explicit retry)
draft|scheduled ──cancel──► cancelled        draft|scheduled|failed|cancelled|published may be deleted (soft delete)
draft ──publish now──► publishing
```
Allowed transitions table (anything else → `INVALID_STATE_TRANSITION`, 409):

| from | to |
|---|---|
| draft | scheduled, publishing, cancelled |
| scheduled | draft, publishing, cancelled |
| publishing | published, partially_published, failed |
| failed | scheduled, publishing |
| partially_published | scheduled, publishing (only failed targets are retried) |
| published, cancelled | — (terminal) |

Post status is derived from target statuses when all targets are terminal: all published → `published`; some → `partially_published`; none → `failed`.
Target statuses: `pending`, `publishing`, `published`, `failed`, `cancelled`, `needs_review`.

### Provider capabilities (returned by `GET /social/providers`)
```
ProviderCapabilities { CanPublishText, CanPublishImage, CanPublishVideo, CanSchedule (native), CanDelete, CanAnalytics,
                       MaxTextLength, MaxMediaCount, RequiresApproval bool, Notes string }
```
* **LinkedIn** – real adapter (OAuth 2.0 auth-code, OpenID `userinfo`, `/rest/posts`, `w_member_social`). Text + image; video marked unsupported in MVP; company pages need Marketing Developer Platform approval (`RequiresApproval=true`, flagged in Notes). Analytics: false.
* **Telegram** – real adapter (Bot API). Not OAuth, and **one platform-wide bot** (`TELEGRAM_BOT_TOKEN`) serves every tenant. Because the bot is shared, knowing a channel's `@username` proves nothing: "the bot is admin there" is true for every user's channel. A chat is therefore connected only by **proof of control**: the user asks SocialOS for a one-time link code, adds the bot as admin with "Post messages" and posts the code in the chat. The bot sees the post (`channel_post` / group `message`), the backend matches the code to its owner, re-checks the bot's rights on that chat (`getChat`/`getChatMember`) and creates the `social_account` for **that user only**. There is no endpoint that takes a chat name or id. The bot token is never stored per account (account metadata keeps a `token_ref` only). Text, image, video, delete. Analytics: false.
* **mock** – deterministic in-memory/DB-less provider used by tests and `SOCIAL_MOCK_PROVIDERS=true` dev mode. Clearly labelled.
* instagram, facebook, tiktok, youtube, x, threads, pinterest – **registered as `unsupported` stubs** returning `PROVIDER_NOT_AVAILABLE` and capabilities with `RequiresApproval=true`. Not pretended to work.

## 3. Database (PostgreSQL 16)

All tables: `id uuid pk default gen_random_uuid()`, `created_at`, `updated_at timestamptz default now()` (omitted below for brevity). Every user-owned table has `user_id` and every query filters by it (tenant isolation).

```mermaid
erDiagram
  users ||--o{ sessions : has
  users ||--o{ social_accounts : owns
  users ||--o{ posts : owns
  users ||--o{ media : uploads
  users ||--o{ api_keys : owns
  users ||--o{ mcp_connections : owns
  users ||--o{ telegram_link_codes : requests
  users ||--o{ email_tokens : "is mailed"
  users ||--o{ data_exports : requests
  telegram_link_codes }o--o| social_accounts : "connected"
  users ||--o{ audit_logs : generates
  social_accounts ||--|| oauth_credentials : "encrypted tokens"
  posts ||--o{ post_targets : has
  social_accounts ||--o{ post_targets : "published via"
  posts ||--o{ post_media : attaches
  media ||--o{ post_media : "used in"
  post_targets ||--o{ scheduled_jobs : schedules
  post_targets ||--o{ publication_attempts : records
  api_keys ||--o| mcp_connections : "credential of"
  social_accounts ||--o{ analytics : measures
  post_targets ||--o{ analytics : measures
```

```sql
users(id, email citext unique, password_hash, display_name, status ['active','disabled','deleted'],
      email_verified_at null, terms_accepted_at null, terms_version, plan default 'free', deleted_at null)   -- migration 00003
email_tokens(id, user_id FK, purpose ['verify_email','reset_password'], token_hash unique, email citext, expires_at, used_at null)   -- migration 00003; only the SHA-256 of the token is stored; a newer token of the same purpose retires older ones
data_exports(id, user_id FK, status ['pending','running','ready','failed','expired'], storage_key, size_bytes, error_code, expires_at null)   -- migration 00003, used by the export work; one active export per user
account_deletions(id, user_id (no FK), requested_at, purged_at null, counts jsonb)   -- migration 00003, used by the deletion work; no PII on purpose
sessions(id, user_id, token_hash unique, csrf_token, expires_at, user_agent, ip)
oauth_states(id, user_id, provider, state_hash unique, code_verifier, redirect_after, expires_at, used_at)
social_accounts(id, user_id, provider, provider_account_id, username, display_name, avatar_url,
                scopes text[], metadata jsonb, status ['active','expired','revoked','error'], connected_at,
                unique(user_id, provider, provider_account_id))
telegram_link_codes(id, user_id FK, code_hash unique, expires_at, used_at, chat_id, social_account_id FK null, created_at, updated_at)   -- migration 00002; only the SHA-256 of the code is stored
oauth_credentials(id, social_account_id unique, access_token_enc, refresh_token_enc, expires_at, refresh_expires_at, key_version)
posts(id, user_id, title, status, scheduled_at, published_at, created_by ['user','api_key'], created_by_ref, deleted_at, quota_counted_at null)   -- quota_counted_at: migration 00003, used by the quota work
post_targets(id, post_id, user_id, social_account_id, platform, content, status, external_post_id,
             external_url, published_at, error_code, error_message, idempotency_key unique, attempt_count)
media(id, user_id, kind ['image','video'], mime_type, size_bytes, storage_key, original_name, width, height, status, sha256)
post_media(post_id, media_id, position, primary key(post_id, media_id))
scheduled_jobs(id, post_target_id, run_at, asynq_task_id, status ['pending','enqueued','done','cancelled'], unique(post_target_id) where status in ('pending','enqueued'))
publication_attempts(id, post_target_id, attempt_no, started_at, finished_at, status ['started','succeeded','failed','unknown'],
                     error_code, error_message, response_metadata jsonb, unique(post_target_id, attempt_no))
api_keys(id, user_id, name, prefix, key_hash unique, scopes text[], expires_at, revoked_at, last_used_at)
mcp_connections(id, user_id, api_key_id, name, client_name, last_seen_at, revoked_at)
audit_logs(id, user_id, actor_type ['user','api_key','scheduler','system'], actor_id, actor_label, action, resource_type, resource_id, metadata jsonb, request_id, ip)
analytics(id, user_id, social_account_id, post_target_id null, metric, value bigint, captured_at)   -- MVP: table + endpoint, filled by adapters that CanAnalytics (none yet) and by internal counters
```
Indexes: `(user_id, status)`, `(user_id, scheduled_at)`, `post_targets(post_id)`, `scheduled_jobs(run_at) where status='pending'`, `audit_logs(user_id, created_at desc)`, `telegram_link_codes(user_id, created_at desc)`, `telegram_link_codes(user_id, expires_at) where used_at is null`, `telegram_link_codes(expires_at)`, `email_tokens(user_id, purpose, created_at desc)`, `email_tokens(expires_at)`.

## 4. REST API (`/api/v1`)

Error format everywhere:
```json
{"error":{"code":"SOCIAL_ACCOUNT_EXPIRED","message":"LinkedIn authorization has expired","request_id":"…"}}
```
Codes: `VALIDATION_ERROR 400`, `UNAUTHENTICATED 401`, `FORBIDDEN 403` (also missing scope: `INSUFFICIENT_SCOPE`; also `EMAIL_NOT_VERIFIED` when the server enforces email verification and the owner has not verified, see Auth), `NOT_FOUND 404`, `INVALID_STATE_TRANSITION 409`, `CONFLICT 409`, `RATE_LIMITED 429`, `SOCIAL_ACCOUNT_EXPIRED 422`, `PROVIDER_NOT_AVAILABLE 501`, `PROVIDER_ERROR 502`, `INTERNAL 500`.
Pagination: `?limit=&cursor=` → `{"items":[…],"next_cursor":null|"…"}`. Times are RFC 3339 UTC.

### Auth
`POST /auth/register {email,password,display_name}` · `POST /auth/login` · `POST /auth/logout` · `GET /me`

Email verification and password recovery (mail goes through the queued mail port, D-006; links carry the token in the URL fragment, `{WEB_BASE_URL}/verify-email#token=…` and `/reset-password#token=…`):

| Endpoint | Auth | Result |
|---|---|---|
| `POST /auth/verify-email {token}` | public | `200 {"email_verified":true}`; unknown, used or expired token: `400 VALIDATION_ERROR` "link is invalid or has expired" |
| `POST /auth/verify-email/resend` | session | `202 {"status":"accepted","delivery":"log"\|"smtp"}`; already verified `409`; inside the 60 s cooldown `429` |
| `POST /auth/password/forgot {email}` | public | always `202 {"status":"accepted","delivery":"log"\|"smtp"}`, identical for known, unknown and malformed addresses (it only enqueues mail) |
| `POST /auth/password/reset {token,password}` | public | `204`; sets the password, **revokes every session**, marks the email verified; invalid token `400`; a weak password is rejected without burning the token |
| `POST /auth/password/change {current_password,new_password}` | session | `204`; revokes every session **except the current one**; wrong current password `400` |

Tokens: 32 random bytes, stored as SHA-256, single use (atomic consume), TTL 48 h for verification and 30 min for reset; a new token retires the older ones of the same purpose. Public endpoints sit behind the auth limiter plus a mail limiter (1 per minute, burst 3, per client). Audit actions: `user.email_verified`, `user.password_reset`, `user.password_changed` (metadata never contains tokens). A `password_changed` notice mail follows both password changes.

**Gating.** Verification is enforced only when `MAIL_PROVIDER=smtp` (otherwise nobody could receive the link). An unverified owner then gets `403 EMAIL_NOT_VERIFIED` on: starting an OAuth or Telegram connection, creating a scheduled post and scheduling, publishing or retrying a post, and creating API keys or MCP connections (sessions and API keys alike). Drafts, reading and everything else stay available. The scheduler and system actors are never blocked, so work a verified user already queued still publishes. Existing users start unverified (`email_verified_at` NULL); with `MAIL_PROVIDER=log` nothing is gated.

`GET /me` (also the body of register and login) adds `user.email_verified` (bool), `user.plan`, and top level `verification_enforced` (bool) and `mail_delivery` (`"log"` or `"smtp"`).

Browser mutating requests need header `X-CSRF-Token` (value returned by `GET /me` / login in `csrf_token`, also in cookie `socialos_csrf`). API-key requests are exempt.

### Social
`GET /social/providers` (capabilities + configured flag) · `GET /social/accounts` · `GET /social/{provider}/connect` (302 to provider; `?redirect=` allow-listed) · `GET /social/{provider}/callback` · `POST /social/telegram/connect` (non-OAuth; no body, mints a link code) · `GET /social/telegram/connect/{id}` (link status) · `GET /social/accounts/{id}` · `DELETE /social/accounts/{id}` (disconnect)

### Telegram linking (proof of control)
Both endpoints are **session only** (API keys get 403) and the POST is CSRF-protected and rate-limited per user (the auth limiter, key prefix `link:`).

* `POST /social/telegram/connect` takes **no chat**. It creates a link code bound to the session user and returns `201 {id, code, expires_at, bot_username, instructions}`, e.g. `SOS-7KQ2M9XA`. A `{chat}` body, if a stale client still sends one, is ignored: it can never name a chat.
* `GET /social/telegram/connect/{id}` returns `{status: "pending"|"connected"|"expired", account?}`. It is tenant-scoped: another user's id, an unknown id and a malformed id are all `404 NOT_FOUND`.

Code: `SOS-` plus 8 characters drawn by rejection sampling from `crypto/rand` over an unambiguous 31-character alphabet (no `0 O 1 I L`), about 40 bits. Only its SHA-256 is stored (`telegram_link_codes.code_hash`, unique), so a database read does not reveal usable codes. TTL 15 minutes, **single use**, at most 3 active codes per user (creating a fourth retires the oldest).

Redemption (`application/accounts.HandleChatUpdate`, the single entry point for both intake modes):

1. The adapter (`adapters/telegram`) parses the update into a provider-neutral `ChatMessage`. Only `channel_post` and group/supergroup `message` count; edits, private chats, bot senders, other update types and messages posted on behalf of another chat are ignored.
2. The message must consist of the code and nothing else (surrounding whitespace and letter case are ignored). Anything else is dropped before hashing.
3. The hash is looked up; the code must be unused and unexpired. Otherwise nothing happens: **no reply**, only a debug log without the code.
4. In a group or supergroup anyone can post, so the **sender must be the creator or an administrator** (`getChatMember`), or an anonymous admin posting as the group. A channel post is trusted by construction: only admins can post there.
5. The existing `VerifyChat` checks run on that chat id: the bot must be an administrator and, for channels, hold "Post messages". If not, the code is **not** burned, so the user can fix the rights and post it again.
6. One transaction upserts the `social_account` for the code's owner, writes an audit entry (actor `system`, source `telegram`) and marks the code used with the `chat_id` and account id.
7. Best effort: `deleteMessage` removes the code message from the chat.

Update intake, `TELEGRAM_UPDATES_MODE` (both modes call the same method):

| Mode | How |
|---|---|
| `polling` (default) | A goroutine in the worker long-polls `getUpdates` (`timeout=25`, `allowed_updates=channel_post,message,my_chat_member`). A Redis lease (`SET NX PX`, renewed while polling) guarantees that only one worker polls; the next offset lives in Redis and is committed only while the lease is held. Errors back off exponentially with jitter (honouring `retry_after`); an update that keeps failing is dropped after 5 attempts. A 409 means a webhook is registered: the poller logs how to remove it and never deletes it on its own. |
| `webhook` | `POST /api/v1/webhooks/telegram`, outside the session/CSRF stack. The `X-Telegram-Bot-Api-Secret-Token` header is compared with `TELEGRAM_WEBHOOK_SECRET` in constant time, else `401`. The body is limited to 256 KiB and, once the secret is verified, the answer is always `200 {"ok":true}` (a garbage body must not make Telegram redeliver forever). The route is mounted only in this mode (404 otherwise). Register it with `make -C backend telegram-set-webhook`, which calls `setWebhook` with `secret_token`. |

Security properties: user B cannot connect user A's channel without posting a code B created in it, so B must be able to post in that chat; B polling A's link id gets 404; expired, reused and unknown codes connect nothing. Residual risk: anyone who is an administrator of a chat where the bot is an administrator can link it to their own account. That is the proof of control the design relies on.

### Posts
`POST /posts {title?, content, social_account_ids[], media_ids[]?, targets?:[{social_account_id, content}], scheduled_at?}` → creates **draft** (or scheduled when `scheduled_at` given and `schedule:true`)
`GET /posts?status=&from=&to=&limit=&cursor=` · `GET /posts/{id}` (includes targets, media, attempts) · `PATCH /posts/{id}` (draft/scheduled only) · `DELETE /posts/{id}`
`POST /posts/{id}/publish` · `POST /posts/{id}/schedule {scheduled_at}` · `POST /posts/{id}/cancel` · `POST /posts/{id}/retry` · `GET /posts/{id}/status`

### Media
`POST /media` (multipart `file`; ≤ 100 MB video / 10 MB image; MIME sniffed server-side; allow-list jpeg/png/webp/gif, mp4/quicktime) · `GET /media` · `GET /media/{id}` (includes short-lived `url`) · `DELETE /media/{id}`

### Analytics & dashboard
`GET /analytics?from=&to=` · `GET /dashboard/summary` → `{connected_accounts, scheduled_posts, drafts, published_this_month, failed, upcoming:[…], recent:[…]}`
`GET /audit-logs?limit=&cursor=&action=` (session only; `action` keeps one action, e.g. `mcp.tool_call` for agent actions)

### Developer
`GET/POST /developer/api-keys {name, scopes[], expires_at?}` (raw key returned once) · `DELETE /developer/api-keys/{id}` (revoke)
`GET/POST /developer/mcp-connections {name, scopes[]}` (creates key, returns raw key once + ready-to-paste config) · `DELETE /developer/mcp-connections/{id}` · `GET /developer/usage`

### Audit actions and MCP headers
Audit actions: `user.registered|login|logout`, `social_account.connected|disconnected|expired`, `post.created|updated|deleted|scheduled|unscheduled|cancelled|publish_requested|retried|completed`, `post_target.published|failed|needs_review`, `media.uploaded|deleted`, `api_key.created|revoked`, `mcp_connection.created|revoked`, `api_key.request` (any API-key request without a tool name), **`mcp.tool_call`** (one MCP tool call, see D-007).

An API-key request that carries `X-MCP-Tool: <tool_name>` (must match `^[a-z_]{1,64}$`, otherwise it is dropped and the request is recorded as `api_key.request`) is audited as one `mcp.tool_call` row instead. Metadata is an allow-list: `tool`, `method`, `route` (pattern, never the raw path), `status`, `error_code`, `target_ids` (UUID URL params `id` / `*_id`), `client` (key label, `MCP: <name>` for MCP connections), `credential_id` (key id, never the key), `via_gateway`; the row's `ip` is the client IP. Headers, query strings, bodies and tokens never reach the log.

| Header | Direction | Meaning | Trusted when |
|---|---|---|---|
| `X-MCP-Tool` | MCP server → API | tool name behind the request | descriptive only (any key holder can set it), API-key actors only |
| `X-SocialOS-Gateway` | MCP server → API | the shared secret `MCP_GATEWAY_SECRET` | constant-time equal; empty secret = never |
| `X-SocialOS-Client-IP` | MCP server → API | end client's IP as the MCP server saw it (same right-to-left `TRUST_PROXY`/`TRUSTED_PROXIES` rule as the API) | only together with a valid gateway header, else ignored. Stdio mode sends neither |

Env: `MCP_GATEWAY_SECRET` (backend and mcp share it; min 32 chars, empty = disabled; `deploy/init-env.sh` generates it). The usage counters (`GET /developer/usage`) count both `api_key.request` and `mcp.tool_call`.

### Ops
`GET /health` (liveness) · `GET /ready` (Postgres + Redis + S3) · `GET /metrics` (Prometheus text, basic counters, optionally token-protected)

### Scopes
`social:read` (accounts, providers) · `posts:read` · `posts:write` (create/update drafts, cancel) · `posts:schedule` · `posts:publish` (**sensitive**) · `posts:delete` (**sensitive**) · `social:disconnect` (**critical**) · `media:write` · `analytics:read`.
Browser sessions have all scopes. API keys carry only granted scopes; `publish`, `delete`, `disconnect` are never in default sets; UI shows them under a "Dangerous" heading and requires confirmation. API keys can never create/revoke API keys, or change password (session-only).

## 5. MCP tools

Transport: Streamable HTTP at `POST /mcp` with `Authorization: Bearer sk_live_…` (also stdio mode for local clients using `SOCIALOS_API_KEY`). Tool list is **filtered by the key's scopes** (fetched from `GET /me`); every call is additionally enforced by the REST API.

| Tool | Scope | Risk | REST |
|---|---|---|---|
| list_social_accounts | social:read | safe | GET /social/accounts |
| get_social_account | social:read | safe | GET /social/accounts/{id} |
| list_posts | posts:read | safe | GET /posts |
| get_post | posts:read | safe | GET /posts/{id} |
| get_post_status | posts:read | safe | GET /posts/{id}/status |
| get_analytics | analytics:read | safe | GET /analytics |
| create_draft `{content, social_account_ids[], media_ids?, title?, per_platform_content?}` | posts:write | safe | POST /posts |
| update_post | posts:write | low | PATCH /posts/{id} |
| schedule_post `{post_id, scheduled_at}` | posts:schedule | medium | POST /posts/{id}/schedule |
| cancel_scheduled_post | posts:write | medium | POST /posts/{id}/cancel |
| publish_post `{post_id, confirm: true}` | posts:publish | sensitive | POST /posts/{id}/publish |
| delete_post `{post_id, confirm: true}` | posts:delete | sensitive | DELETE /posts/{id} |
| disconnect_account `{account_id, confirm: true}` | social:disconnect | critical | DELETE /social/accounts/{id} |

Dangerous tools require an explicit `confirm: true` argument and carry MCP annotations (`destructiveHint`, `readOnlyHint`). Every REST call made via an API key writes an audit log with actor `api_key` / key name; tool calls are recorded as `mcp.tool_call` with the tool name (the MCP server sends `X-MCP-Tool` on every tool call, see §4 "Audit actions and MCP headers").

## 6. Scheduler / publishing flow & idempotency

1. `schedule`: tx { post → `scheduled`, `scheduled_at`; per target insert `scheduled_jobs(pending)` } → enqueue Asynq task `publish:target` `ProcessAt(scheduled_at)`, `TaskID = target.id + ":" + run_at`, `MaxRetry(5)`, `Retention`. Store `asynq_task_id`, mark `enqueued`.
2. Worker handler `publish:target(target_id)`:
   1. tx: lock target `FOR UPDATE SKIP LOCKED`; skip if status ∈ {published, cancelled}; if `external_post_id` set → mark published, return. Move post `scheduled → publishing` (idempotent), target → `publishing`, insert attempt `started` (`attempt_no = attempt_count+1`).
   2. Load account; if `expires_at` within 5 min → `provider.RefreshToken`; failure → account `expired`, target `failed` with `SOCIAL_ACCOUNT_EXPIRED` (non-retryable).
   3. Load media (presigned/streamed from S3), call `provider.PublishPost(ctx, req{IdempotencyKey: target.id})`.
   4. Success: in one tx set `external_post_id`, `published`, attempt `succeeded`, recompute post status. Failure: classify retryable (network, 5xx, 429) vs permanent; retryable → attempt `failed`, return error so Asynq retries with exponential backoff (`30s·2^n`, jitter, max 5); permanent or retries exhausted → target `failed`.
   5. Crash between provider success and DB commit: attempt row remains `started`. On the next try the worker sees a stale `started` attempt → marks it `unknown` and, if provider supports `GetPost`/lookup by idempotency key (Telegram: none; LinkedIn: none) sets target `needs_review` instead of re-posting blindly, unless the adapter declares `SafeToRetryAfterUnknown`. This prefers a missed post over a duplicate and is visible in UI.
3. `cancel` before run: target/jobs → `cancelled`, Asynq task deleted; worker also re-checks status so a late job is a no-op.
4. **Reconciler** (worker, every minute): finds `scheduled_jobs` `pending|enqueued` with `run_at < now() - 1m` without a live task and re-enqueues; also recovers `publishing` targets stuck > 15 min.

## 7. OAuth flow
```
Browser ─GET /social/linkedin/connect (session)─► API: state=random 32B, store sha256(state)+user_id+PKCE verifier (10 min TTL)
        ◄─302 provider authorize?client_id&redirect_uri&state&scope─
User consents → provider ─302 /api/v1/social/linkedin/callback?code&state─► API
API: lookup state hash, ensure unused+unexpired AND belongs to the session user, mark used → exchange code → fetch profile
     → upsert social_account(user_id, provider, provider_account_id) → encrypt+store tokens → audit → 302 {WEB_BASE_URL}/accounts?connected=linkedin
```
Errors redirect to `/accounts?error=<code>`; tokens never logged; provider id (not token) identifies the account.

## 8. Backend layout (hexagonal, differences from the brief explained)
```
backend/
  cmd/api  cmd/worker  cmd/migrate  cmd/telegram (setWebhook / deleteWebhook / webhookInfo)
  internal/
    domain/        user socialaccount post media job apikey audit linkcode      (entities, state machine, errors; no I/O)
    application/   auth accounts posts scheduler media developer analytics audit   (use cases + port interfaces)
    infrastructure/ postgres redis storage crypto queue(asynq) clock
    adapters/      provider(interface, registry, capabilities)  linkedin telegram(+update parsing, poller, webhook helpers) mock stubs(instagram tiktok youtube twitter …)
    transport/     http(handlers, router, dto) middleware(requestid, logging, auth, csrf, ratelimit, cors, recover)
    config/ observability/
  migrations/  Dockerfile  Makefile
```
Differences: `social` ports live in `application` (not `domain`) because they are I/O contracts; `observability` added; one module hosts API+worker to share domain code.

## 9. Runnable without production credentials
Everything (api, worker, mcp, frontend, Postgres, Redis, MinIO) runs locally; with `SOCIAL_MOCK_PROVIDERS=true` the "mock" provider appears as a connectable network so the full acceptance flow works end-to-end. LinkedIn/Telegram code is real but exercised in tests against `httptest` fakes; live use needs `LINKEDIN_CLIENT_ID/SECRET` (+ redirect URI registered, product "Share on LinkedIn" / "Sign In with LinkedIn using OpenID Connect") and `TELEGRAM_BOT_TOKEN` (bot added as channel admin).

## 10. Phases
1 skeleton+contract · 2 auth/users/accounts/Postgres · 3 LinkedIn · 4 Telegram · 5 posts+scheduler+Asynq · 6 media+MinIO · 7 MCP · 8 frontend dashboard/composer/calendar · 9 developer portal · 10 tests · 11 docker+docs · 12 E2E verification.
