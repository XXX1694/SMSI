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

## D-009: Connect with a pasted token needs the critical `social:connect` scope (2026-10-09)

**Context.** Webhook URLs, app passwords and API keys are bearer secrets. Networks that use them (Discord, Mastodon,
Bluesky, Slack, Dev.to, VK) need one generic way to connect. Per-provider endpoints mean N handlers and N forms, and
metadata cannot hold the secret because the account API returns it.

**Decision.** One port and one route. A provider declares `connect_method: token` and its form (`connect_fields`);
`POST /social/accounts/token` validates the fields, calls the adapter's `Verify` (live whoami, 10 s) and stores the
credential in the encrypted vault, reusing the OAuth upsert and audit path. The scope `social:connect` is **critical**
and never part of a default set: an agent that can connect accounts can attach a credential it chose, so it is granted
on purpose. Browser sessions always may connect. Once server-side approvals exist, a key-initiated connect should need
the owner's approval like other dangerous actions. MCP gets no connect tool.

**Alternatives.** An endpoint per provider (rejected: duplication). Secrets in `metadata` (rejected: returned by the
API). Allowing sessions only (rejected: it blocks the API-key testing flow in PLATFORMS).

**Consequences.** Static tokens are never refreshed; a revoked one marks the account `expired` and the user repeats the
POST. Adapters must keep secrets out of the profile; the use case refuses a profile that contains them.

## D-010: User-supplied hosts are fetched through an SSRF-safe client (2026-10-09)

**Context.** Mastodon, Misskey and WordPress take a host from the user, so the server would fetch a user-chosen URL.

**Decision.** Such adapters use `safehttp.NewSafeClient`: https only; the client resolves the host itself, refuses
loopback, private, link-local (including the 169.254.169.254 metadata address), CGNAT, multicast, unspecified and other
reserved addresses in IPv4, IPv6 and IPv4-mapped/NAT64/6to4 forms, dials the vetted address (no second DNS lookup, so
no rebinding) and re-checks it in the dialer `Control`; redirects only to the same host, at most three; responses over
1 MB fail instead of being truncated; no proxy environment variables; timeouts on dial, TLS, headers and the whole
request. Adapters with a fixed host set `AllowedHosts` instead. Every adapter takes an injected `http.Client` for tests.

**Consequences.** A self-hosted instance on a private network cannot be connected. Accepted: SocialOS runs on a shared
host next to other services (D-001).

## D-011: Bound password hashing in memory (2026-10-09)

**Context.** Every argon2id hash used 64 MiB and nothing limited how many ran at once. The production API container has a
160 MB cap and the auth limiter allows bursts of 10 per IP, so a few parallel logins or registrations could get the API
OOM-killed.

**Decision.**
- The `crypto` hasher runs at most `PASSWORD_HASH_CONCURRENCY` (default 2) operations at once **and** keeps their combined
  argon2 memory within `PASSWORD_HASH_MEMORY_MIB` (default 48). Memory is read from the PHC string, so an old 64 MiB hash
  is heavier than the whole budget and verifies alone; two new 19 MiB hashes run together. Worst case is therefore one
  64 MiB verification, or about 38 MiB for two new hashes, never 128 MiB.
- A caller waits for capacity until its context ends or 5 s pass, then gets `429 RATE_LIMITED` ("server is busy, retry
  shortly") with `Retry-After: 5`. No new error code. An unknown email gets the same answer as a known one while the
  hasher is saturated, so load does not reveal which addresses exist; a busy hasher is never reported as a wrong password.
- New hashes use the OWASP minimum argon2id configuration, m=19 MiB, t=2, p=1
  ([Password Storage Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html)).
  Stored hashes keep verifying because their parameters are read from the hash.
- A successful login with a hash made under other parameters rehashes the password, best effort: a failure is logged and
  never fails the login. The write is conditional on the hash that was verified, so a password reset or change that lands
  meanwhile is not overwritten.

Rejected: raising the container limit (hides the burst), and a new 503 code (needs contract, MCP and docs changes for no
client benefit).

**Consequences.** Under a burst, excess logins wait up to 5 s and are then told to retry. Until old hashes are upgraded,
each old-hash login occupies the whole budget for its duration. The first login after deploy costs one extra hash.

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
share; only when the host is under pressure and SocialOS contributes (slice anon ≥ 300 MB, ≥ 40% of a CPU or ≥ 10 MB/s IO) it
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

## D-013: Dangerous actions by API keys need the owner's approval, enforced in the API (2026-10-09)

**Context.** Until now an agent was asked to pass `confirm: true` for publish, delete and disconnect. The flag is an
argument the agent writes itself, so it proves nothing, and it only existed in the MCP server: a plain REST call with the
key skipped it.

**Decision.** The application layer decides. When a request authenticated by an API key attempts a dangerous action
(publish now, retry now, delete a post, disconnect an account, connect with a pasted token (D-009), or schedule less than
`AGENT_MIN_SCHEDULE_LEAD` (5 m) ahead, which is publish-now in disguise) and the key's `dangerous_policy` is `approve`
(the default for every new and existing key), the API answers `428 APPROVAL_REQUIRED` with an `approval_id` and stores a
pending approval. Only a browser session can approve or deny it. The agent then repeats the identical call with the
header `X-Approval-Id`. An approval belongs to one user and key, covers one action on one target with one payload
(fingerprint: post id and `updated_at`, the requested time, a hash of the whole create body or of the connect fields),
expires after `APPROVAL_TTL` (10 m) and is spent in the same transaction as the action, so a failed action does not burn
it and two racing calls cannot both use it. The scope check still comes first. The `confirm` flag is gone from MCP.
Sessions, the scheduler and system actors never need approval; `dangerous_policy: trusted` (chosen when the owner creates
the key) skips it for that key on purpose. Anything that does not match gets a fresh `428` and no hint why (no oracle on
other tenants' ids); an approval the owner denied gets `403` so the agent stops. A repeated ask returns the open
approval; at most 10 are open per key (a noisy key cannot block the others), checked and inserted under a per-key lock
so parallel calls cannot overshoot. The gate answers in two steps: inside the use case's transaction `Require` consumes
a matching approval or returns `ApprovalNeeded`; after the transaction rolled back, `Open` stores the pending row and
answers 428. A row written inside the transaction would be erased by that rollback, and a second connection opened
meanwhile could exhaust the pool under load. An edit of a post is approved as the post it will become (not as the
field the key named), and any key edit of a post running within the lead needs approval. The owner sees the full text,
per-network texts and media. A token connect is fingerprinted with an HMAC under an HKDF subkey of the encryption key.
Decided and expired approvals are purged after 30 days. Created, approved, denied and used are audited.

**Alternatives.** A server-minted confirmation token: the agent can fetch and present it itself, so it proves no more than
`confirm: true`. A per-key allow flag only: no per-action guarantee. Enforcing in the MCP server: bypassed by curl.
Approval id in the body: DELETE has none, a header works for every route.

**Consequences.** Breaking for API-key integrations: dangerous calls now take two steps and a human, so unattended
publishing needs a `trusted` key (opt-in, shown as such) or a scheduled post at least 5 minutes ahead. The approval is
spent before the live check of a token connect, so a rejected credential needs a new approval. Whoever holds the owner's
browser session can approve. `trusted` is visible: the key list shows it as a badge, and choosing it at creation needs a separate explicit
confirmation under the "Dangerous" heading. Changing the policy of an existing key and an OAuth-grant policy come later.
