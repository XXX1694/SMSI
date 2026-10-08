# Changelog

All notable changes to SocialOS are listed here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and versions follow [Semantic Versioning](https://semver.org/). Pushing a tag `vX.Y.Z` builds the images and publishes a
GitHub Release whose notes are the matching section of this file (see "Releasing" in [`deploy/README.md`](deploy/README.md)).

## [Unreleased]

### Added

- Email verification, password reset and password change. Mailed links carry a single-use token in the URL fragment (48 h to verify, 30 min to reset). A reset signs out every session; a change keeps the current one. API keys and MCP connections are revoked only if you tick "Also revoke all API keys and MCP connections".
- When `MAIL_PROVIDER=smtp`, unverified accounts get `403 EMAIL_NOT_VERIFIED` on connecting networks, scheduling, publishing, editing scheduled posts and creating API keys. With the log provider nothing is restricted and the dashboard says mail is off. Existing accounts start unverified.
- New pages `/verify-email`, `/forgot-password` and `/reset-password`, an email banner, and a Password section in Settings.
- Database migration `00003` also prepares plans, quotas, data export and account deletion; it needs no manual step.

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
