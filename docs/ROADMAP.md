# Roadmap

Status of the path from MVP to a public release. Milestones and issues on GitHub track the details. The decisions behind
the choices are in [DECISIONS.md](DECISIONS.md).

Legend: ✅ done · 🔄 in progress · ⏳ next · ⛔ blocked (the reason is given)

## Done

- ✅ MVP: Go backend (hexagonal, Postgres, Redis/Asynq, S3), MCP server with 13 scoped tools, Next.js dashboard.
- ✅ Real LinkedIn and Telegram adapters (tested against fakes). Telegram channel ownership is proven by a one-time code.
- ✅ CI, image release to GHCR (amd64 + arm64), SSH deploy workflow, Pages workflow, Dependabot.

## 1. Stabilization ✅

- ✅ Dependabot PRs consolidated and merged (#9); majors adapted or deliberately ignored with a reason.
- ✅ Actions on Node 24, runners pinned to ubuntu-24.04; main CI has no warnings.
- ✅ Frontend tests pass on Node 22 and 25+.
- ✅ Pages enabled; site, docs and demo verified with Playwright at desktop and mobile widths (polish items in #16).
- ✅ Found on the way: posts could publish up to 1 s early (#12).

## 2. Production 🔄

- ✅ "Host proxy" deploy mode for a shared server, plus memory limits (D-001, #13); host Caddy import installed on the server with validation and rollback.
- ✅ Pull-based deploys from GitHub Releases (D-002, #13, #28); release tags protected by a ruleset.
- 🔄 First deploy on `*.sslip.io` (D-003): HTTPS certificates issued for app/api/mcp/s3; v0.1.0 deploy and the Telegram webhook are next.
- ✅ Rate limiting that trusts only the proxy's client IP, IPv6 /64 buckets, bounded keys (#10).
- ⏳ Off-site backups and a tested restore.
- 🔄 Uptime checks with incident issues (#26, merges after the first deploy); error monitoring waits for a Sentry DSN (D-005).

## 3. Real integrations

- 🔄 Telegram live on the production domain: bot token on the server, bot verified, webhook free (D-004).
- ⛔ LinkedIn OAuth live: waiting for the owner to create the LinkedIn developer app ([guide](integrations/linkedin.md)).
- ⏳ The acceptance scenario passes in production with real networks.

## 4. Ready for users

- ✅ Transactional mail port, SMTP + log (D-006, #17).
- 🔄 Email verification, password reset and change (#29, security fixes in progress). Delivery in production waits for a real domain (Resend needs a verified sender).
- ⏳ Account deletion and data export.
- ⏳ Per-user quotas.
- ✅ Terms and Privacy pages; registration needs `accept_terms` (D-016).
- ⏳ Onboarding.

## 5. MCP as the headline feature

- ⏳ OAuth 2.1 for MCP clients, next to API keys.
- ⏳ Ready-made configs for Claude Desktop, Claude Code and Cursor.
- ✅ Audit of every MCP tool call (D-007, #18).
- ✅ Server-side approval of dangerous actions (D-013): API, MCP and the dashboard Approvals page.
- ⏳ Publish to npm and MCP registries (only after the owner's go-ahead).

## 6. Security before release

- ⏳ Threat model and an OWASP ASVS L1 review, written up in `docs/SECURITY.md`.
- ✅ Dependency, image, secret and code scanning in CI: govulncheck, npm audit, gitleaks, Trivy, CodeQL (#19).
- ✅ X-Forwarded-For trust and `post_targets` scoping fixed (#10).
- ⏳ Frontend CSP; bundled-Caddy IPv6 buckets (#11).

## 7. Design and product

- 🔄 Positioning, competitors and a prioritised backlog in [PRODUCT](PRODUCT.md); the "Now" features are issues #75–#82.
- ⏳ Landing page; one design language; empty states; mobile layout.
- 🔄 More networks: research of every platform's current API and a tiered rollout plan (token-only first) in progress.
- ⏳ Ideas with honest effort estimates: an AI writing assistant, analytics, a Flutter client.

## 8. Release

- 🔄 Semver tags and CHANGELOG ready (#28); v0.1.0 is next. Self-host guide: deploy/README sections 13–15.
- ⏳ Announcement drafts (nothing is published without the owner's "yes").
