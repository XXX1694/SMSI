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

## D-006: Transactional mail through a queued, provider-agnostic port (2026-10-09)

**Context.** Email verification, password reset and account notices need mail. Sending inline in the request is slow, makes
register fail when the relay is down, and lets response timing reveal whether an address exists.

**Decision.** Services depend on `port.MailQueue`, which enqueues a `mail:send` Asynq task (MaxRetry 5, Retention 0,
so one-time links do not linger in Redis). The worker runs `port.Mailer`: an SMTP adapter (STARTTLS required on 587,
or implicit TLS on 465; it never authenticates or sends in clear text) or a log adapter (masked recipient, body only when
`APP_ENV=development`). Templates are embedded, English, plain, rendered to text plus HTML. `MAIL_PROVIDER` defaults to `log`.

**Consequences.** Any relay works; Resend is the intended one. Resend needs a verified sending domain, so production
mail stays off while the host uses sslip.io (D-003). `smtp` with a missing field fails at startup; `log` in production
only logs a warning. A rendered body (with its link) sits in Redis until the worker sends it.

## D-007: Audit MCP tool calls in the backend (2026-10-09)

**Context.** Agents act on a user's accounts through MCP. The user needs to see what an agent did, with the outcome,
and the log must not be forgeable by the client or leak content.

**Decision.** The backend writes the audit row, not the MCP server. The MCP server tags every tool call with
`X-MCP-Tool: <name>`; the API-key audit middleware records a request with a valid tag as one `mcp.tool_call` row (instead
of `api_key.request`) with the status and error code the backend itself produced. Metadata is an allow-list (tool,
route pattern, status, error code, URL ids, key label and id, client IP). The client IP comes from
`X-SocialOS-Client-IP` only when `X-SocialOS-Gateway` equals `MCP_GATEWAY_SECRET` (constant-time compare); otherwise it
is the caller's own address. The tool name is descriptive, not a security input.

**Alternatives.** The MCP server POSTs its own audit events: it would assert the outcome itself and cost an extra call
per tool. Trusting the client-IP header without a secret: anyone could forge the address in the log.

**Consequences.** Calls that bypass the MCP server (plain REST with a key) stay `api_key.request`. A key holder can
mislabel the tool name of their own calls; the route and status are still the backend's. `MCP_GATEWAY_SECRET` is a new
shared secret that must be set on both services, otherwise the audit IP is the MCP server's.

## D-008: Email verification and password reset use single-use hashed tokens in the URL fragment (2026-10-09)

**Context.** Users need to prove they own an address and recover a lost password. Verification must not lock out
self-hosters who have no mail relay, and the endpoints must not reveal which addresses are registered.

**Decision.**
- One `email_tokens` table serves both purposes. A token is 32 random bytes; only its SHA-256 is stored. Redeeming it is
  one atomic `UPDATE … WHERE used_at IS NULL AND expires_at > now() RETURNING`, so a link works once even under
  concurrent clicks. Lifetimes are 48 h (verify) and 30 min (reset). Issuing a new token retires the older ones, and a
  user gets at most 10 tokens per purpose per rolling 24 h; beyond that, requests are dropped silently (same `202`).
- Links use the URL fragment, which browsers send neither to the server nor in `Referer`, so tokens stay out of access
  logs; the frontend reads the fragment and clears it with `history.replaceState`.
- `POST /auth/password/forgot` only validates the address, enqueues an `auth:forgot` task (Retention 0) and answers `202`
  with a fixed body. The lookup, the 60 s cooldown, the cap and the mail all happen in the worker, so the request does
  the same work for every address and its status, body and timing do not depend on whether the address exists.
- A reset signs out every session and marks the address verified; a change keeps only the current session. Neither
  touches API keys or MCP connections unless the caller sets `revoke_keys` (opt-in checkbox, audited as
  `keys_revoked`). The UI and the `password_changed` mail say which happened and link to the Developer page. Keys
  survive by default because agents run on them, but a reset after a suspected break-in should offer the revoke.
- The mail task carries the token row id, never the raw token. If the final retry fails, the worker retires that token,
  so a task left in the archive holds a dead link.
- Verification is enforced (`403 EMAIL_NOT_VERIFIED`) only when `MAIL_PROVIDER=smtp`; with the log provider nothing is
  gated and the UI says mail is off. The check also runs where the actor says nothing about the owner: completing an
  OAuth or chat-link connection checks the owner's row, and editing an already scheduled post needs a verified owner,
  so switching mail on later does not leave unverified users with working paths.

Rejected: signed JWT links (cannot be revoked one by one), tokens in the query string (end up in logs), sending the
reset mail inline (its timing would reveal the address), and enforcing verification unconditionally (blocks every
self-hoster without SMTP).

**Consequences.** `register` still answers `409` for an existing address, so existence can be probed there (accepted,
the same as most sign-up forms). Existing users are unverified and must verify once mail is switched on. The rendered
mail, with its link, sits in Redis until the worker sends it (D-006), and the forgot task holds the address until
it runs. The mail limiter is in memory and per instance.

## D-012: SocialOS gets at most half of the shared host, enforced by a systemd slice and a load-shedding guard (2026-10-09)

**Context.** Production shares a 2 vCPU / 2 GB VPS with another production service (D-001). Containers could swap, the
default per-container task limit (2264) times seven exceeds the host's thread limit (about 15000), and dockerd/containerd
are unbounded (containerd peaked at 963 MB during the first image pull). The other service's units, Caddy and the host
configuration are not ours to change.

**Decision.** All SocialOS containers run in `socialos.slice` (compose `cgroup_parent` in host-proxy mode; Docker already
uses the systemd cgroup driver on cgroup v2, so no daemon change or restart): `CPUQuota=100%`, `CPUWeight=50`,
`MemoryHigh=616M`, `MemoryMax=664M`, `MemorySwapMax=0`, `TasksMax=512`, a 60 MB/s read cap and no write cap. Each container
gets `memswap_limit = mem_limit` and `pids_limit`; backend and worker keep 160m with `GOMEMLIMIT=100MiB` for upload buffers.
Drop-ins give dockerd and containerd `CPUQuota=25%`, `CPUWeight=50` and a soft `MemoryHigh` (192M / 128M), with no boot
ordering. Slice plus daemons: 984M, half of the RAM. A guard timer reads host and per-service pressure (PSI) and SocialOS's
share; only when the host is under pressure and SocialOS contributes (slice ≥ 400 MB, ≥ 40% of a CPU or ≥ 10 MB/s IO) it
stops the worker, then the non-essential containers, and starts what it stopped after calm runs (level 2 with a doubling
backoff). It alerts but never acts on the other service. A Caddy pre-check quarantines a SocialOS snippet that would stop
Caddy from starting.

**Alternatives.** `daemon.json` `cgroup-parent`: catches `docker run` too, but needs a Docker restart. Per-container caps
that add up to the slice: leaves the backend too little room for uploads. A slice of 848M (no overcommit): about 56% of the
RAM. `MemoryMax` on containerd: could kill the shims and orphan running containers. `IOWeight` alone: no effect with the
`none` IO scheduler. A write cap: throttled writeback can stall ext4 journal commits and the other service's fsync.
`After=caddy irbisa` on Docker: nice at boot, but a cycle if either is ever ordered after Docker. Pausing the worker: it
would freeze holding database locks. A guard that sheds on any pressure: it would stop SocialOS for pressure it does not
cause. A loop-mounted filesystem as a hard disk quota: needs a data migration; deferred. A drop-in on `caddy.service`:
edits a unit that belongs to the other service.

**Consequences.** The per-container caps (784m, 848m during a deploy) overcommit the 664M slice: under a simultaneous
peak, the OOM kill hits SocialOS inside its slice, never the other service. Under real contention the slice gets a third
of the CPU. A shed stops publishing (posts go out late) and, at level 2, the UI and MCP until the host has been calm for
10 minutes or more; autoupdate pauses meanwhile. The read cap is a placeholder until the disk is measured. Standalone mode
is unchanged. Not solved here (the other service's owner): `caddy.service` has `Restart=no`, and journald has no size cap.
