# SocialOS: notes for AI agents

SocialOS schedules and publishes social posts. AI agents drive it through an MCP server, and people use it through a web UI.
The contract (API, MCP, DB schema, post state machine, idempotency) is in `docs/ARCHITECTURE.md`. Read it before you change
behaviour. Decisions are logged in `docs/DECISIONS.md`, and status is tracked in `docs/ROADMAP.md`.

## Layout

| Path | What | Stack |
|---|---|---|
| `backend/` | REST API, worker, migrations, Telegram tool (`cmd/api`, `cmd/worker`, `cmd/migrate`, `cmd/telegram`) | Go 1.24, hexagonal: `internal/domain`, `internal/application` (ports in `port/` and per-service `ports.go`), `internal/adapters`, `internal/infrastructure`; Postgres (pgx), Redis + Asynq, S3 (minio-go) |
| `mcp/` | MCP server, 13 tools filtered by API-key scope | TypeScript, MCP SDK, Streamable HTTP (stateless) + stdio |
| `frontend/` | Dashboard | Next.js 15 (App Router), Tailwind 3.4 |
| `site/` | Landing, docs, in-browser demo for GitHub Pages | static build (`site/build.mjs`) |
| `deploy/` | Production: `docker-compose.prod.yml`, Caddy, `deploy.sh`, `backup.sh`, `init-env.sh` | docker compose |

## Commands

```bash
make test                 # unit tests: backend + mcp + frontend (no services needed)
make lint                 # all linters (golangci-lint, eslint --max-warnings 0, tsc)
make up                   # whole stack in Docker; make dev = infra in Docker, apps on the host
make test-integration     # Go integration/e2e; needs TEST_DATABASE_URL, TEST_REDIS_URL
make acceptance           # MVP flow against a running stack (mcp/scripts/acceptance.mjs)
cd frontend && npm run build:demo && cd ../site && npm run build && npm run check   # Pages site
```

- CI uses Node 22 (local Node 25+ works: `frontend/tests/setup.ts` restores jsdom storage).
- Browser scripts (`npm run smoke`, `frontend/scripts/e2e-real.mjs`) take `CHROMIUM_PATH`. On macOS, point it at Google Chrome.
- If `:5432` or `:6379` is taken on the host, run throwaway Postgres 16 / Redis 7 containers on other ports for integration tests.

## Conventions

- Code, commits, docs and UI copy are in English. Commits follow Conventional Commits (`feat(mcp): ...`) and carry no Co-Authored-By trailer.
- Flow: branch → PR with a description → green CI → self-review → merge. Auth, crypto, tenancy, uploads and deploy also need a security review.
- A task is not done until it has tests and green CI. UI changes also need Playwright screenshots that someone has looked at.
- Be honest about capabilities: if a network's API needs app review or is not available, say so in `capabilities` and in the UI. Never fake success.
- Tenant isolation: every repository query on user data filters by `user_id`. Add a case to `backend/internal/e2e/security_test.go` for new resources.
- Each new decision with alternatives gets a short entry in `docs/DECISIONS.md`.

## Do not

- Commit secrets anywhere, including docs, tests and examples. Production secrets live only in `.env` on the server (mode 600). GitHub holds no app secrets.
- Publish anything externally without the owner's explicit "yes": social posts, npm, MCP registries or announcements. The only exception is the private test Telegram channel.
- Touch anything on the production host outside `/opt/socialos` and the single `import` line in the host's Caddyfile. Another project runs there.
- Run `git stash` (the stash is shared between worktrees), force-push `main`, or rewrite published history.
- Weaken a test or lower a threshold to get CI green.
