# Roadmap

Status of the path from MVP to a public release. Milestones and issues on GitHub track the details. The decisions behind
the choices are in [DECISIONS.md](DECISIONS.md).

Legend: ✅ done · 🔄 in progress · ⏳ next · ⛔ blocked (the reason is given)

## Done

- ✅ MVP: Go backend (hexagonal, Postgres, Redis/Asynq, S3), MCP server with 13 scoped tools, Next.js dashboard.
- ✅ Real LinkedIn and Telegram adapters (tested against fakes). Telegram channel ownership is proven by a one-time code.
- ✅ CI, image release to GHCR (amd64 + arm64), SSH deploy workflow, Pages workflow, Dependabot.

## 1. Stabilization 🔄

- ⏳ Merge or close the Dependabot PRs (rebase onto main; adapt the major bumps).
- ⏳ Remove the deprecation warnings in Actions.
- ⏳ Make frontend tests pass on Node 25+ (jsdom `localStorage` is shadowed by Node's experimental web storage).
- ⏳ Enable Pages and verify the site and the demo on https://xxx1694.github.io/SMSI/.

## 2. Production 🔄

- ⏳ "Host proxy" deploy mode for a shared server, plus memory limits (D-001).
- ⏳ Pull-based deploys from GitHub Releases (D-002).
- ⏳ First deploy on `*.sslip.io` with HTTPS (D-003), healthchecks, Telegram webhook.
- ⏳ Rate limiting that trusts only the proxy's client IP.
- ⏳ Off-site backups and a tested restore.
- ⏳ Error monitoring and uptime checks.

## 3. Real integrations

- ⏳ Telegram live on the production domain (existing bot, D-004).
- ⛔ LinkedIn OAuth live: waiting for the owner to create the LinkedIn developer app (step-by-step guide to follow).
- ⏳ The acceptance scenario passes in production with real networks.

## 4. Ready for users

- ⏳ Email verification and password reset (SMTP adapter, D-005).
- ⏳ Account deletion and data export.
- ⏳ Per-user quotas.
- ⏳ Terms and Privacy pages.
- ⏳ Onboarding.

## 5. MCP as the headline feature

- ⏳ OAuth 2.1 for MCP clients, next to API keys.
- ⏳ Ready-made configs for Claude Desktop, Claude Code and Cursor.
- ⏳ Server-side confirmation of dangerous actions, and audit of every MCP call.
- ⏳ Publish to npm and MCP registries (only after the owner's go-ahead).

## 6. Security before release

- ⏳ Threat model and an OWASP ASVS L1 review, written up in `docs/SECURITY.md`.
- ⏳ Dependency, image and secret scanning in CI.
- ⏳ Known items: X-Forwarded-For trust in the rate limiter, `post_targets` lookups without `user_id`, frontend CSP.

## 7. Design and product

- ⏳ Positioning and landing page; one design language; empty states; mobile layout.
- ⏳ Ideas with honest effort estimates: more networks as their APIs allow, an AI writing assistant, analytics, a Flutter client.

## 8. Release

- ⏳ Semver tags and CHANGELOG; a GitHub Release with GHCR images; a self-host guide.
- ⏳ Announcement drafts (nothing is published without the owner's "yes").
