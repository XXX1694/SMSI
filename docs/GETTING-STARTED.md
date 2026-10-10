# Getting started

Run Steerpost locally, configure it, apply migrations and run the test suites. To put it on a server, follow
[`deploy/README.md`](../deploy/README.md). The design behind all of this is in [ARCHITECTURE](ARCHITECTURE.md).

## Local setup

**One command (Docker):**

```bash
make up                # = make env (creates .env with a fresh ENCRYPTION_KEY) + docker compose up -d --build
open http://localhost:3000
```

Services:

| Service | Address |
|---|---|
| frontend | :3000 |
| backend | :8080 |
| mcp | :3333/mcp |
| postgres | :5432 |
| redis | :6379 |
| minio | :9000, console :9001 |
| worker | none exposed |
| migrate | one-shot |

Use `make logs`, `make ps` and `make down` to watch, inspect and stop the stack.

**Without production credentials** the whole MVP still works. `SOCIAL_MOCK_PROVIDERS=true`, the default in `.env.example`, adds a "Mock Network" provider with a real OAuth-style redirect flow. Connect it twice with different hints (`/api/v1/social/mock/connect?account=linkedin`, then `?account=telegram`) to get two accounts. The mock is clearly labelled and refused in `APP_ENV=production`.

**Apps on the host, infrastructure in Docker** (hot reload): run `make dev` and follow the printed commands.

## Environment variables

[`.env.example`](../.env.example) is the compose-level file. Every backend option, with comments, is in [`backend/.env.example`](../backend/.env.example). The most important ones:

| Variable | Purpose |
|---|---|
| `DATABASE_URL`, `REDIS_URL` | Postgres and Redis |
| `ENCRYPTION_KEY` | base64 of 32 bytes (`openssl rand -base64 32`); encrypts OAuth tokens. Keep it stable |
| `S3_ENDPOINT`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `S3_BUCKET`, `S3_PUBLIC_ENDPOINT` | Object storage (MinIO / R2 / S3) |
| `API_PUBLIC_URL`, `WEB_BASE_URL`, `MCP_PUBLIC_URL`, `CORS_ALLOWED_ORIGINS` | Public URLs and the CORS allow-list (`*` is rejected) |
| `LINKEDIN_CLIENT_ID`, `LINKEDIN_CLIENT_SECRET` | LinkedIn app |
| `TELEGRAM_BOT_TOKEN` | Telegram bot (one per deployment, shared by all users) |
| `TELEGRAM_UPDATES_MODE` | How the bot receives chat posts: `polling` (default, the worker long-polls) or `webhook` |
| `TELEGRAM_WEBHOOK_SECRET` | Webhook mode only: secret Telegram echoes in `X-Telegram-Bot-Api-Secret-Token` (1-256 chars of `A-Z a-z 0-9 _ -`; 16+ in production) |
| `MAIL_PROVIDER` | `log` (default: mail is written to the log and email verification is not enforced) or `smtp` |
| `SOCIAL_MOCK_PROVIDERS` | Mock network (development only) |
| `COOKIE_SECURE`, `METRICS_TOKEN`, `RATE_LIMIT_*` | Hardening |
| `TRUST_PROXY`, `TRUSTED_PROXIES` | Behind a reverse proxy: `TRUST_PROXY=true` makes the backend read `X-Forwarded-For`, but only from peers in `TRUSTED_PROXIES` (CIDRs; default loopback + private ranges) and only the right-most hop outside that set counts. Invalid CIDRs stop startup |

`JWT_SECRET` is intentionally absent. Sessions are opaque and revocable server-side, which is simpler and safer than JWT refresh logic for an MVP. Real secrets are never committed: `.env` is git-ignored.

## Database migrations

SQL migrations live in `backend/migrations` (goose format) and are embedded in the binaries.

```bash
make migrate                         # docker: one-shot migrate service
make -C backend migrate              # host: uses backend/.env DATABASE_URL
```

The full schema, with its tables and indexes, is in section 3 of [ARCHITECTURE](ARCHITECTURE.md). Every table has UUID primary keys and `created_at`/`updated_at`. One user can own many accounts on the same platform, enforced by `unique(user_id, provider, provider_account_id)`.

## Running the components individually

| Component | Command | Notes |
|---|---|---|
| Backend API | `make -C backend run-api` | `:8080`; `GET /health`, `GET /ready` (Postgres, Redis, S3), `GET /metrics` |
| Worker | `make -C backend run-worker` | Publishes due targets, retries with exponential backoff (max 5), runs a reconciler every minute and, in `polling` mode, long-polls Telegram for link codes (one worker at a time, via a Redis lease). Health on `:8081` |
| Frontend | `cd frontend && npm ci && npm run dev` | `:3000`. `npm run mock-api` serves the contract on `:8080` for UI work without the backend |
| MCP | `cd mcp && npm ci && npm run build && SOCIALOS_API_URL=http://localhost:8080 npm start` | HTTP mode. Add `--stdio` with `SOCIALOS_API_KEY` for stdio mode |

## Tests

```bash
make test               # Go unit tests + MCP tests + frontend lint/typecheck/tests: no services, no credentials
make test-integration   # Go integration + e2e against real Postgres/Redis (TEST_DATABASE_URL, TEST_REDIS_URL)
make acceptance         # the MVP acceptance flow against a running stack (API + worker + MCP)
cd frontend && node scripts/e2e-real.mjs   # browser e2e against a running stack (Playwright)
```

The suites cover these areas:

- post state transitions;
- OAuth state validation: wrong user, reused or expired state;
- token refresh before publishing;
- scheduler, retries, backoff and the retry cap;
- idempotency: a re-run job never posts twice, and a crash after the provider call becomes `needs_review`;
- tenant isolation;
- scope authorization (`INSUFFICIENT_SCOPE`);
- CSRF, media validation and the uniform error format;
- LinkedIn and Telegram adapters against `httptest` fakes;
- MCP tool filtering by scope, confirm flags, error mapping and the SDK client over Streamable HTTP.

No test needs real OAuth credentials.

## Acceptance criterion

`make acceptance` runs this flow end to end against a running stack:

1. Register a user.
2. Connect two accounts.
3. Create an MCP connection without publish permissions.
4. An agent connects and sees only the allowed tools.
5. The agent calls `list_social_accounts`.
6. The agent calls `create_draft`.
7. The agent calls `schedule_post` for a few seconds ahead.
8. The worker publishes the post at the right time.
9. The API refuses a direct publish with the agent's key (`403 INSUFFICIENT_SCOPE`).
10. The dashboard counts the publication, and the audit log shows both the agent and the scheduler.

With real LinkedIn and Telegram credentials, the same flow runs with those providers instead of the mock.

## Production notes

- Set `APP_ENV=production`. Startup then refuses insecure cookies, the in-memory storage driver and the mock provider.
- Point `S3_*` at Cloudflare R2 or S3, and put the API, MCP and frontend behind TLS (`mcp.example.com`, …). [`deploy/`](../deploy/README.md) does exactly that with Caddy and automatic certificates.
- The rate limiter is per instance. Move it to Redis when you scale horizontally.
- Metrics are Prometheus text format. OpenTelemetry tracing can wrap the existing request-ID and correlation-ID middleware.

## CI/CD and deployment

GitHub Actions workflows and a single-server deployment kit:

| Workflow | Runs on | What it does |
|---|---|---|
| [`ci.yml`](../.github/workflows/ci.yml) | pull requests, pushes to `main`, weekly (Mon 03:00 UTC), manual | Backend (golangci-lint with gofmt and vet, govulncheck, unit tests, integration + e2e on real Postgres 16 and Redis 8), MCP and frontend (typecheck, lint, test, build, first-load JS budget, `npm audit`), site (demo and site build, link check, browser smoke test, `npm audit`; on `main` it also deploys GitHub Pages), E2E (build the three images once, compose stack with `mcp/scripts/acceptance.mjs`, Trivy on the same images), repo-lint (actionlint, shellcheck, deploy tests, compose and Caddy checks, gitleaks), CodeQL (Go and TypeScript). Pull requests run only the jobs whose paths changed (a change to `ci.yml` counts as everything); `main`, the schedule and manual runs run all of them; a newer push cancels the pull-request run it replaces. The single required check is the aggregator job **CI ok** |
| [`release.yml`](../.github/workflows/release.yml) | green CI on `main`, tags `v*`, manual | Pushes multi-arch (amd64 + arm64) images to `ghcr.io/xxx1694/socialos-{backend,mcp,frontend}` tagged `sha-<7 hex>`, `main` and, for `vX.Y.Z` tags, `X.Y.Z` / `X.Y` / `X`. For a tag it then creates the GitHub Release (notes from [`CHANGELOG.md`](../CHANGELOG.md)) once all three images exist: that release is what servers with pull-based updates follow |
| [`deploy.yml`](../.github/workflows/deploy.yml) | after a main release, manual | SSHes to the server, uploads [`deploy/`](../deploy/), runs `deploy.sh <tag>` (pull, migrate, restart, wait for `/ready`, automatic rollback). Skipped unless the `DEPLOY_*` secrets exist; runs in the `production` environment, where you can add required reviewers |
| [`uptime.yml`](../.github/workflows/uptime.yml) | at :07, :22, :37, :52 (best-effort), manual, `repository_dispatch` `uptime-check` | Probes the public endpoints (repository variables `UPTIME_URLS`, `API_PUBLIC_URL`, `MCP_PUBLIC_URL`) and opens an incident issue when one is down |

**First-load JS budget.** After the production build, the frontend job runs `npm run budget` (in `frontend/`, after `npm run build`). It sums the gzip (level 6) of every script a first visit to a route downloads, layouts included (Next's printed table omits them), and compares it with [`frontend/bundle-budget.json`](../frontend/bundle-budget.json), in kB. It also fails if shell text (`skipToContent`) returns to the root layout chunk or Radix Dialog lands in an auth route. An over-budget route prints a table and exits 1. Raising a budget is a deliberate edit of that JSON in the same pull request, with the reason in its description and the CHANGELOG; lower a budget when a route shrinks (the ratchet of [AGENTS.md](../AGENTS.md) section 4). Background: issue #180 and D-021 in [DECISIONS](DECISIONS.md).

[`.github/dependabot.yml`](../.github/dependabot.yml) opens grouped weekly update pull requests for Go, npm, Actions and Docker images.

**Set up:** repository variables `API_PUBLIC_URL` (`https://api.<domain>`) and `MCP_PUBLIC_URL` (`https://mcp.<domain>/mcp`), then, for automatic deploys, the secrets `DEPLOY_HOST`, `DEPLOY_USER`, `DEPLOY_SSH_KEY` (and `DEPLOY_KNOWN_HOSTS`). The frontend image **bakes in** those URLs at build time, so they have to be set *before* the release you deploy; see [`deploy/README.md`](../deploy/README.md#frontend-urls-read-this-once).

**Deploy by hand, from zero to HTTPS** (DNS records, server preparation, first deploy, migrations, Telegram webhook, LinkedIn redirect URL, daily backups with an optional encrypted off-site copy and a restore drill, rollback by image tag): [`deploy/README.md`](../deploy/README.md). On a server where a reverse proxy already owns ports 80/443, use its [host-proxy mode](../deploy/README.md#13-behind-an-existing-reverse-proxy-host-proxy-mode); a server can also [pull releases by itself](../deploy/README.md#14-pull-based-updates) instead of receiving pushes. To exercise the same stack locally, `make up` and `make acceptance` do what the e2e job does.

## Website and demo

The landing page, the docs and a **browser-only demo of the real app** are built by the `site` job of [`ci.yml`](../.github/workflows/ci.yml) on every pull request that touches them and published to GitHub Pages by its `pages` job after each push to `main`: <https://xxx1694.github.io/steerpost/>, with the demo at <https://xxx1694.github.io/steerpost/demo/>. The docs are rendered from the repository's markdown: the root [`README.md`](../README.md), this file, [`integrations/README.md`](integrations/README.md), [`API.md`](API.md), [`ARCHITECTURE.md`](ARCHITECTURE.md) and [`mcp/README.md`](../mcp/README.md). The sources are in [`site/`](../site/) (plain HTML and CSS plus a small Node build script) and in the demo mode of [`frontend/`](../frontend/). Pages must be enabled once (Settings, Pages, Source: GitHub Actions).

**How the demo works.** It is the production frontend built with `NEXT_PUBLIC_DEMO=true` as a static export. A typed in-browser backend ([`frontend/src/lib/demo/`](../frontend/src/lib/demo/), a port of `frontend/scripts/mock-api.mjs` that shares the real API types) answers every API call: a demo user who is already signed in, LinkedIn, Telegram and mock accounts, drafts, scheduled, published and failed posts across this and next month, API keys, MCP connections and an audit log. The scheduler is simulated client-side, state lives in `localStorage` (the banner's **Reset** restores the seed) and nothing leaves the browser. Actions that need a real network, such as LinkedIn OAuth and the Telegram link code, are simulated and say so. The demo is not part of the production build: `npm run build` is unchanged.

**Build and preview it locally** (Node 20+; `build` and `build:demo` share `frontend/.next`, so run them one after the other):

```bash
(cd frontend && npm ci && npm run build:demo)   # static demo -> frontend/out
(cd site && npm ci && npm run build)            # site + demo  -> site/dist
(cd site && npm run preview)                    # http://localhost:4173/steerpost/
(cd site && npm run check && npm run smoke)     # link check + browser smoke test (needs Chrome or Chromium)
```

The landing page screenshots are real captures of the demo and are committed; after a visible UI change, re-capture them with `(cd site && npm run screenshots)`. The site serves from `/steerpost/` by default; `SITE_BASE` (site) and `NEXT_PUBLIC_BASE_PATH` (demo, `<SITE_BASE>demo`) change that, which the workflow derives from the repository name.
