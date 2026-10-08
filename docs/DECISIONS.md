# Decisions

A short log of decisions that had alternatives. Newest last. Each entry gives the context, the decision and the
consequences. The entries are amended rather than deleted: when a decision is superseded, a new entry says so.

## D-001: Production runs on a shared VPS, behind the host's existing reverse proxy (2026-10-09)

**Context.** The production server is a small VPS (2 vCPU, 2 GB RAM, 40 GB disk) that already runs another service.
That service's Caddy (systemd, not Docker) owns ports 80 and 443. `deploy/` assumed SocialOS owns the whole host and
runs its own Caddy container on 80/443.

**Decision.** SocialOS lives in `/opt/socialos` and runs in Docker. Its containers publish ports on `127.0.0.1` only.
The host Caddy gets one line, `import /opt/socialos/caddy/*.caddy`, and our site blocks are in that file. `deploy/`
gains a "host proxy" mode (compose override + Caddy snippet) next to the existing standalone mode. The existing
mode stays the default for self-hosters. Containers get memory limits sized for a 2 GB host.

**Consequences.** Nothing else on the host changes. The Caddyfile is validated before every reload and backed up, and
the reload is rolled back if the other site stops answering. RAM is the tightest resource, so we measure it after the
first deploy. A larger server would cost money and needs the owner's approval.

## D-002: Secrets live only on the server, and deploys are pulled, not pushed (2026-10-09)

**Context.** The owner wants every key on the server and none in GitHub. `deploy.yml` deploys over SSH, which needs a
private key in GitHub Secrets.

**Decision.** Production secrets are only in `/opt/socialos/.env` (mode 600, root). A systemd timer on the server
polls the repository's latest GitHub Release (public API, no token). When the tag changes, it runs `deploy.sh <tag>`,
which pulls the public GHCR images, migrates, waits for `/ready` and rolls back on failure. `deploy.yml` stays for
self-hosters who prefer push deploys. It is skipped here, because the `DEPLOY_*` secrets do not exist.

**Consequences.** CI has no write access to the server, and a compromised GitHub token cannot reach production.
A deploy starts within the polling interval (5 min) after a release is published. Only tagged releases reach
production.

## D-003: No paid domain yet, so production uses sslip.io hostnames (2026-10-09)

**Context.** Budget is 0, and there is no domain or DNS access.

**Decision.** `DOMAIN=<ip-with-dashes>.sslip.io`, which gives `app.`, `api.`, `mcp.` and `s3.` hostnames that resolve to
the server with no DNS setup. Moving to a real domain later only changes `.env` (DOMAIN, COOKIE_DOMAIN, public URLs),
the release variables `API_PUBLIC_URL`/`MCP_PUBLIC_URL`, the LinkedIn redirect URL and the Telegram webhook.

**Consequences.** sslip.io is not on the Public Suffix List, so Let's Encrypt rate limits are shared with every
sslip.io user. Certificate issuance is checked after the first deploy. If Let's Encrypt refuses, the options are
Caddy's secondary ACME issuer (ZeroSSL) or a real domain. A real domain is the first thing to buy once there is a budget.

## D-004: Telegram uses the owner's existing bot token, which will not be rotated (2026-10-09)

**Decision (owner).** Production uses the existing bot. The token is supplied straight to the server's `.env`. It never
goes into the repository, GitHub or chat logs. Production receives updates by webhook (`TELEGRAM_UPDATES_MODE=webhook`).

**Consequences.** Before the webhook is registered, `getWebhookInfo` is checked so that we do not take over updates
from another consumer of the same bot.

## D-005: Free tiers only, with graceful fallbacks for services that need the owner's accounts (2026-10-09)

**Decision.** Transactional mail goes through a provider-agnostic SMTP adapter. In dev, mail is written to the log;
Resend is the intended production provider. Errors go to a Sentry-compatible SDK that is a no-op without a DSN.
Uptime is checked by a scheduled GitHub Actions workflow. There is no product analytics at launch. Off-site backups
are designed for any S3-compatible bucket.

**Consequences.** Every feature can ship and be tested without the accounts. Each one becomes live in production as
soon as its key is in `.env`. Until then, the UI and the docs state honestly what is off.
