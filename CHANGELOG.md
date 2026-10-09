# Changelog

All notable changes to SocialOS are listed here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and versions follow [Semantic Versioning](https://semver.org/). Pushing a tag `vX.Y.Z` builds the images and publishes a
GitHub Release whose notes are the matching section of this file (see "Releasing" in [`deploy/README.md`](deploy/README.md)).

## [Unreleased]

### Security

- The generated Claude Desktop config no longer runs `npx -y socialos-mcp`: that npm package does not exist, and whoever registered the name would have received users' API keys. Configs, the MCP connections page and the docs now use Claude Desktop connectors or the `mcp-remote` bridge pinned to an exact version, and a regression test rejects `socialos-mcp` and unpinned `npx` packages.

### Added

- Email verification, password reset and password change. Mailed links carry a single-use token in the URL fragment (48 h to verify, 30 min to reset). A reset signs out every session; a change keeps the current one. API keys and MCP connections are revoked only if you tick "Also revoke all API keys and MCP connections".
- When `MAIL_PROVIDER=smtp`, unverified accounts get `403 EMAIL_NOT_VERIFIED` on connecting networks, scheduling, publishing, editing scheduled posts and creating API keys. With the log provider nothing is restricted and the dashboard says mail is off. Existing accounts start unverified.
- New pages `/verify-email`, `/forgot-password` and `/reset-password`, an email banner, and a Password section in Settings.
- Database migration `00003` also prepares plans, quotas, data export and account deletion; it needs no manual step.
- Connect a network with a pasted credential: `POST /api/v1/social/accounts/token` and the provider capability fields `connect_fields`, `max_image_bytes` and `requires_title`. It needs the new critical API-key scope `social:connect`, which is never in a default set (D-009).
- "Connect with a token" on the Accounts page: Discord, Mastodon and Bluesky open a form built from the provider's `connect_fields` (secrets as password inputs with show/hide, never stored or put in a URL), with the how-to linked and plain-English errors per field. The demo and mock API answer the same fields and the token endpoint.
- Posts are checked against the stricter of the network limits and the account's own limits, and networks that need a title reject posts without one.
- SSRF-safe HTTP client for hosts users supply (D-010).
- Discord: connect a channel with its webhook URL and publish text and up to 10 images, or delete a post. The URL is stored encrypted and never returned; a timeout after sending goes to review instead of risking a duplicate. See [`docs/integrations/discord.md`](docs/integrations/discord.md).
- Connect fields have a `secret` flag (`connect_fields[].secret` in `GET /social/providers`) for credentials that are not of kind `secret`, such as a webhook URL; forms must render them as password inputs.
- Mastodon (and API-compatible Fediverse servers): connect with an instance URL and an access token, publish text and images (public), delete. Limits are read from the instance. A repeated publish after a timeout is safe because of the `Idempotency-Key`. See `docs/integrations/mastodon.md`.
- Bluesky: connect with a handle and an app password, publish text with link and hashtag facets and up to 4 images, delete, and resolve unknown outcomes through a deterministic record key. See `docs/integrations/bluesky.md`.
- Stub notes for Reddit, Medium and Hashnode; the X note now says the API is pay-per-use.
- Sharing a host safely (host-proxy mode): every SocialOS container runs in the systemd slice `socialos.slice` (1 CPU, 664 MB, no swap, 512 tasks for the whole stack), may not swap and has a process cap; drop-ins cap dockerd and containerd; a guard timer stops the SocialOS worker, then the worker, MCP and UI, only when the host is under pressure and SocialOS is a real contributor (anonymous memory, not page cache, or its own CPU or IO), and resumes them once the host is calm; it also alerts on disk, data budget and the other services' health. A Caddy pre-check keeps a broken SocialOS snippet from stopping the host's Caddy. Runbook: `deploy/host-proxy/apply-guardrails.md`.

- Approvals for dangerous actions (D-013): publishing now, retrying now, deleting a post, disconnecting an account, connecting with a pasted token, or scheduling less than 5 minutes ahead (`AGENT_MIN_SCHEDULE_LEAD`) with an API key or an MCP connection answers `428 APPROVAL_REQUIRED` and waits for you. Approve or deny in the dashboard under **Approvals** (`GET /approvals`, `POST /approvals/{id}/approve|deny`); the agent repeats the same call with `X-Approval-Id` and it works once. New settings `APPROVAL_TTL`, `APPROVAL_MAX_PENDING` (per key), `APPROVAL_RETENTION`, `AGENT_MIN_SCHEDULE_LEAD` (also gates edits of posts that run soon); new audit entries `approval.requested|approved|denied|used`.
- API keys have a `dangerous_policy`: `approve` (default, also for existing keys) or `trusted` (set when creating the key, skips approvals).

### Changed

- **Breaking for API-key integrations:** dangerous actions made with a key now need your approval (see above). The MCP tools `publish_post`, `delete_post` and `disconnect_account` no longer take `confirm`; they take an optional `approval_id`, and `schedule_post` and `update_post` take one too. An agent that only schedules at least 5 minutes ahead is not affected.
- Password hashing is bounded in memory: at most `PASSWORD_HASH_CONCURRENCY` (default 2) hashes run at once within `PASSWORD_HASH_MEMORY_MIB` (default 48) of argon2 memory, new hashes use argon2id m=19 MiB, t=2, p=1 (OWASP), and existing hashes are upgraded at the next successful login. Under a burst, logins and registrations may get `429 RATE_LIMITED` and should retry.
- The SSRF guard also blocks site-local `fec0::/10`, IPv4-compatible `::/96` and local-use NAT64 `64:ff9b:1::/48` (the whole range is blocked).
- Browser sessions now hold ten scopes (the new `social:connect`); the "Dangerous" list in the dashboard shows it.
- Host-proxy mode: memory caps rebalanced to fit the slice (backend and worker 160m with `GOMEMLIMIT=100MiB`, frontend 160m, postgres 112m, minio 80m, mcp 56m, redis 32m, migrate 64m). Automatic updates do not deploy while the guard has shed load.

### Fixed

- Cancelling, unscheduling or publishing a post while the worker publishes it can no longer deadlock and fail with a 500: the API and the worker now lock rows in the same order (post, then target). A database deadlock or serialization failure that still happens is reported as a retryable `CONFLICT` (#38).
- The Posts date filter now uses the timezone chosen in Settings instead of the browser's, so "From" and "To" cover whole days where you expect them.
- Colour contrast: unsupported-network badges and the dark-mode "Scheduled" badge now meet WCAG AA (axe `color-contrast` is clean in light and dark). The dark accent is slightly lighter, in the app and on the site.
- A failed load no longer looks like an empty list: Posts, post detail and the audit log show a titled error with a retry button, "Load more" failures keep the list on screen, and error messages are plain English (no raw codes, JSON or stack traces).

### Changed

- One shared table for post attempts, analytics, API usage, API keys and the audit log. On phones each row becomes a stacked card showing every column (the Error column is no longer hidden), `/developer` no longer scrolls sideways at 390 px, and wide tables can be scrolled with the keyboard.

## [0.1.0] - 2026-10-09

First release: the MVP, ready to self-host on one server.

### Added

- Web dashboard to connect social accounts, write a post once, tailor it per network and publish it now or schedule it.
- REST API with browser sessions (CSRF-protected) and scoped, revocable API keys (`sk_live_…`, shown once, stored hashed).
- MCP server with 13 tools, filtered by the key's scope, so AI agents can draft, schedule and review posts (Streamable HTTP and stdio).
- Publishing to LinkedIn and Telegram; the other networks are shown honestly as "not available". Connecting a Telegram channel needs a one-time link code.
- Scheduler and worker with idempotent publishing, retries and a clear post state machine; media uploads to S3-compatible storage (bundled MinIO, R2 or S3).
- Production kit in [`deploy/`](deploy/README.md): Docker Compose with automatic HTTPS, `deploy.sh` (migrations, readiness wait, automatic rollback), backups and a runbook.
- Host-proxy mode for a small shared server: loopback-only ports, memory limits sized for about 1 GB, and an installer that adds one `import` line to the host's Caddy with validation, checks and automatic rollback.
- Pull-based updates: a systemd timer deploys the latest GitHub Release when its images exist. It never downgrades and can be switched off with `AUTOUPDATE=false`.
- Transactional mail through a queue, with SMTP and log-only adapters and templates for e-mail verification, password reset, password changed, account deleted and export ready.
- Audit log of every MCP tool call (tool, route, status, target ids, agent), with an "Agent actions" filter in the dashboard; the MCP server and the backend share a gateway secret so the real client IP is recorded.
- Setup guides for [LinkedIn](docs/integrations/linkedin.md) and [Telegram](docs/integrations/telegram.md), and a project site with an in-browser demo.

### Changed

- Dependencies refreshed: Go 1.26, pgx, minio-go, goose and other Go modules, frontend packages, Redis 8; CI actions moved to Node 24 on pinned runners.
- The bundled MinIO now uses a maintained, digest-pinned image because `minio/minio` was removed from Docker Hub.
- `deploy.sh` follows `COMPOSE_FILE`, cleans up only SocialOS images on shared hosts, and exits with 75 when it changed nothing.

### Fixed

- A scheduled post is never published before its `scheduled_at`; an early task is held briefly or handed back to the queue.
- The demo's sample data no longer breaks in the first minutes of a month.

### Security

- The client address is read from the right of `X-Forwarded-For` and only through trusted proxies (`TRUSTED_PROXIES`), so a forged header can no longer earn a fresh login or register rate-limit bucket.
- Rate limits group IPv6 clients by /64 and track a bounded number of keys.
- Post targets are locked and read per user, with tests that no other user can read, change or lock them.
- A Telegram channel can only be connected by proving ownership with a one-time link code.
- CI scans every change: govulncheck, npm audit, gitleaks over the history, Trivy on the images and CodeQL; the runtime images no longer ship npm.

[Unreleased]: https://github.com/XXX1694/SMSI/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/XXX1694/SMSI/releases/tag/v0.1.0
