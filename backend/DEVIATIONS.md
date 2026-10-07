# Deviations, decisions and open gaps

The binding contract is `docs/ARCHITECTURE.md`. Everything below is where the
implementation adds to it, interprets it, or knowingly falls short. Items under
"Unresolved" need a decision from the owner of the other side (frontend, MCP
server or product).

## 1. Additions on top of the contract (compatible, but not in the contract)

| Area | What the backend does | Why |
|---|---|---|
| `GET /me`, `POST /auth/register`, `POST /auth/login` | Canonical body `{user, scopes, csrf_token, auth_type, api_key}` **plus** flat copies `id`, `email`, `display_name` | The MCP server reads `scopes`/`api_key` at top level; the frontend's `Me` type is flat. `csrf_token` is `null` for API keys, `api_key` is `null` for sessions. |
| `POST /developer/api-keys` | Returns `api_key` (record), `key` (raw secret) and `raw_key` (same secret) | Frontend `normalize.ts` and the MCP client look for different names. The raw secret is returned only here, never listed. |
| `POST /developer/mcp-connections` | Returns `connection`, `key`, `raw_key`, `warning` and `config` = `{http, stdio, mcpServers, claude_code}` (JSON objects plus a one-line `claude mcp add` command), all containing the raw key | "Ready-to-paste config" for Claude Code, Cursor and Claude Desktop. |
| `GET /developer/usage` | `{window_days, total_requests, by_key[], by_day[30 days incl. zero days], items}` (`items` = `by_key`) | Frontend charts need `by_day`; older client reads `items`. |
| `GET /analytics` | `{from, to, totals, items, series, note}`; `items` and `series` are the same rows `{platform, metric, day/captured_at, value}` | Contract does not define the shape; frontend expects `items`. |
| `GET /social/providers` | Each item has `id`, `name`, `display_name`, `status` (`supported`/`unsupported`), `supported`, `unsupported`, `configured`, `capabilities` | Frontend needs `id`/`unsupported`; contract names only capabilities and `configured`. |
| Capabilities | Contract keys plus `max_caption_length`, `safe_to_retry_after_unknown`, `connect_method` | Used by validation and the publisher. All keys are snake_case. |
| `GET /posts/{id}` | Always contains `targets`, `media` (objects with a short-lived `url`), `media_ids` and `attempts` (empty arrays when none). List endpoints return `targets` and `media_ids` only | Contract says detail "includes targets, media, attempts". Frontend `Post.media` is optional. |
| `POST /posts/{id}/unschedule` | Scheduled -> draft, removes the queued task | The UI needs a way back to draft that is not `cancel`. |
| `/health`, `/ready`, `/metrics` | Served at the root **and** under `/api/v1` | Reverse proxies that only forward `/api/*`. |
| `/ready` | `{status, checks{postgres,redis,storage: ok\|unavailable}, errors{name: coarse reason}}`; 503 when anything is unavailable. Reasons are one of `connection refused`, `timed out (dependency unreachable or slow)`, `canceled`, `unreachable or misconfigured`; the full error goes to the log only | Makes an S3 outage obvious without leaking hostnames or keys. |
| `/metrics` | Adds `socialos_publish_attempts_total{provider,outcome}` (`published\|failed\|retry\|needs_review`) and `socialos_http_rate_limited_total` | Operational visibility of the scheduler. |
| Nullable strings | `external_post_id`, `external_url`, `error_code`, `error_message` on targets and attempts are always present and `null` when empty (never omitted) | Frontend types declare `string \| null`. |
| Time | Every timestamp is RFC 3339 UTC (`...Z`), regardless of the input offset | Contract, enforced by a test on every e2e response. |

## 2. Decisions where the contract is silent

- **Password policy**: 8-128 characters (the frontend form validates 8). Email is lower-cased and must be a bare address (`Name <a@b>` is rejected).
- **Registration** answers `409 CONFLICT` for an existing email (needed for a usable form); **login** never reveals whether an email exists (identical error, dummy hash verification).
- **Scopes not named in the contract**
  - `GET /media`, `GET /media/{id}` need `posts:read` (media is read as part of posts).
  - `POST /posts/{id}/retry` needs `posts:publish` (it sends again).
  - `GET /dashboard/summary` needs `posts:read`; `GET /analytics` needs `analytics:read`.
  - `GET /social/providers`, `GET /social/accounts*` need `social:read`.
- **Session-only endpoints** (API keys get 403 `FORBIDDEN` even with every scope): `GET /audit-logs`, all of `/developer/*`, `GET /social/{provider}/connect`, `POST /social/telegram/connect`.
- **405 is reported as 404 `NOT_FOUND`** with the standard envelope; the contract has no 405 code.
- **`POST /posts` with an empty body** creates a blank draft (editor autosave). Scheduling or publishing it is rejected with 400 `VALIDATION_ERROR` until it has content and accounts.
- **Post limits**: title 200, content 10 000 characters, 20 accounts, 20 media, `scheduled_at` at most 366 days ahead. Per-network limits come from the provider capabilities.
- **Malformed ids in a path** (`/posts/not-a-uuid`) are `404 NOT_FOUND`, not 400, so id shapes cannot be probed. Another tenant's id is also 404, never 403.
- **Revoking** an API key or MCP connection is idempotent (204 on repeat); revoked and expired keys answer 401.
- **Default API-key scopes** (when `scopes` is omitted) exclude `posts:publish`, `posts:delete` and `social:disconnect`.
- **Scheduled publishing latency**: the worker promotes due tasks every 1 s (Asynq default is 5 s), so a post goes out within about a second of `scheduled_at`.
- **CORS**: `CORS_ALLOWED_ORIGINS` must list explicit origins. `*` is rejected at startup and an empty list disables CORS (the library default would be a wildcard with credentials).
- **Reconciler** (every minute, `RECONCILE_INTERVAL`) re-enqueues overdue jobs that have no live task, recovers `publishing` targets stuck longer than 15 minutes, and sends exhausted targets whose last attempt is `started`/`unknown` to `needs_review` (`OUTCOME_UNKNOWN`) instead of failing them, as the contract prefers a missed post over a duplicate. A `started` attempt younger than 5 minutes (`InFlightWindow`) is treated as owned by a live worker; one provider call is bounded to 3 minutes.
- **Storage outage**: the API starts even when S3 is down (bucket check is retried lazily, `/ready` reports it). An upload during the outage answers `500 INTERNAL` after about 4 s (the cause is in the log, `/ready` shows `storage: unavailable`).
- **Telegram** uses one bot token per deployment (`TELEGRAM_BOT_TOKEN`); the token is never stored per account, never returned and redacted from errors. Accounts keep only `token_ref`.

## 3. Unresolved mismatches with other components

1. **`socialos-mcp` is not published to npm.** The generated stdio config runs `npx -y socialos-mcp --stdio`, which fails until the package exists. Local alternative: `node <repo>/mcp/dist/index.js --stdio` with `SOCIALOS_API_KEY` and `SOCIALOS_API_URL`. The HTTP config (`http://localhost:3333/mcp` or `MCP_PUBLIC_URL`) needs the MCP server running.
2. **Frontend `Capabilities` type is camelCase** (`canPublishText`...). The backend sends snake_case as the contract requires; `frontend/src/lib/normalize.ts` converts, but any consumer that bypasses it will see snake_case keys.
3. **Frontend `CreatedApiKey` is `{key: ApiKey, rawKey}`**; the backend sends `{api_key, key, raw_key}` (see section 1). The normalizer copes (it takes the record from `api_key` and the secret from `raw_key`). `key` therefore means the secret on this endpoint but the record in the frontend type; decide on one shape and drop the aliases.
4. **Frontend `Post.created_by_ref`** (who created it) has no backend field; `created_by` is `user|api_key` only.
5. **Analytics are SocialOS counters only** (published/failed per day and platform). LinkedIn and Telegram expose no analytics API for this use, so `can_analytics` is false for every provider.
6. **No endpoint for changing a password**, although the contract says API keys can never do it. Nothing to protect yet; add it as session-only.
7. **Not implemented (out of scope for the MVP)**: email verification, password reset, session listing/revocation UI, organisations/teams (one user is one tenant), LinkedIn video upload, post editing after publish, webhooks.

## 4. Operational caveats

- **Rate limiting is in memory per API instance.** With N replicas the effective limit is N times higher (put an edge limiter in front or move buckets to Redis).
- **Every API-key request writes an `api_key.request` audit row** (it powers `/developer/usage`). Plan retention if keys are used heavily.
- **LinkedIn and Telegram adapters were tested against local fakes only** (`httptest` servers that mimic the documented endpoints). No live credentials were available, so real-network behaviour, LinkedIn app review (`w_member_social`) and Telegram rate limits are unverified.
- **Unsupported networks** (Instagram, Facebook, TikTok, YouTube, X, Threads, Pinterest) are listed as `status=unsupported`, `requires_approval=true`, with the reason in `notes`. Connecting or publishing returns `501 PROVIDER_NOT_AVAILABLE`; nothing pretends to work.
- **The Dockerfile could not be built in the sandbox** (no Docker daemon). Its steps were run by hand: the same `go build` flags produce static binaries (`ldd`: not a dynamic executable) and `api healthcheck` was exercised against a running instance.
- **`SOCIAL_MOCK_PROVIDERS=true`** registers a fake network. Production startup refuses it, together with `COOKIE_SECURE=false` and `STORAGE_DRIVER=memory`.
