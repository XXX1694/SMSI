# Steerpost: rules for AI agents and contributors

The single source of truth for how work is done in this repository. `CLAUDE.md` is a symlink to this file; do not copy
its content elsewhere. Canon lives in `docs/`: [ARCHITECTURE](docs/ARCHITECTURE.md) (API, MCP, schema, state machine),
[DECISIONS](docs/DECISIONS.md) (why things are the way they are), [ROADMAP](docs/ROADMAP.md) (status),
[PLATFORMS](docs/PLATFORMS.md) (which networks and how). Link to them; do not restate them.

Steerpost schedules and publishes social posts. People use the web UI; AI agents use the MCP server. The product promise is
**an agent can do the work, a human stays in control, and nothing is faked**.

## 1. Map and commands

| Path | What | Stack |
|---|---|---|
| `backend/` | REST API, worker, migrations, Telegram tool (`cmd/api`, `cmd/worker`, `cmd/migrate`, `cmd/telegram`) | Go 1.26, hexagonal: `internal/domain` → `internal/application` (ports) → `internal/adapters`, `internal/infrastructure`, `internal/transport`; Postgres (pgx, goose), Redis + Asynq, S3 (minio-go) |
| `mcp/` | MCP server: tools over the REST API, filtered by key scope | TypeScript, MCP SDK, Streamable HTTP (stateless) + stdio |
| `frontend/` | Dashboard and the in-browser demo (`build:demo`) | Next.js 15 (App Router), Tailwind 3.4 |
| `site/` | Landing, docs and demo for GitHub Pages | static build (`site/build.mjs`) |
| `deploy/` | Production: compose (standalone and host-proxy), Caddy, `deploy.sh`, `autoupdate.sh`, backups | docker compose, bash |

```bash
make test                 # unit tests: backend + mcp + frontend, no services needed (fast; run it before every push)
make lint                 # golangci-lint, eslint --max-warnings 0, tsc
make test-integration     # Go integration/e2e; needs TEST_DATABASE_URL, TEST_REDIS_URL (throwaway containers on free ports)
make acceptance           # the MVP flow against a running stack (mcp/scripts/acceptance.mjs)
make up | make dev        # whole stack in Docker | infra in Docker, apps on the host
bash deploy/tests/run.sh  # deploy script tests (run in an Ubuntu container on macOS)
```

Browser scripts take `CHROMIUM_PATH` (on macOS: Google Chrome). If `:5432`/`:6379` are taken, use throwaway containers on
other ports and remove them afterwards; never touch containers you did not start.

## 2. How work is done

1. **Read before you write.** This file, the relevant part of ARCHITECTURE, then the neighbouring module that solves a similar
   problem; do it the same way. Never guess a signature, a field or a route: open it.
2. **Plan when it is not trivial.** Anything that touches 2+ packages, the schema, auth, the public API/MCP contract, a new
   provider or deploy needs a short written plan first: why, what changes, PR-sized steps, acceptance scenario, risks. Big or
   risky designs go to the `architect` role; the plan names the decision that will land in DECISIONS.
3. **Write the acceptance scenario before the code**: what the user or agent does → what must happen, including the unhappy
   paths (no permission, empty list, provider down, token revoked, quota hit). The regression part becomes a test; the rest is
   checked by hand at the end and the output is shown in the PR.
4. **Cut small.** One PR = one reason to change, reviewable in one sitting (aim for < 400 changed lines of non-test code). If a
   change spreads past ~10 files without a reason, stop and re-plan instead of pushing on.
5. **Reuse before you build.** Search for an existing helper, port, component or test fixture first. A second implementation
   of the same behaviour is a defect: delete the old one in the same PR.
6. **Prove it.** "Should work" is not a result. Run the checks, run the scenario, look at the screenshots, paste the evidence.

Roles (definitions in `.claude/agents/`): `explorer` (haiku, read-only scouting), `implementer` (sonnet, code + tests in its
own worktree), `architect` and `security-reviewer` (opus, read-only), `ui-reviewer` (sonnet, Playwright screenshots). Code is
reviewed by a role that did not write it. Parallel work happens in separate git worktrees under `.worktrees/`; never use
`git stash` (it is shared across worktrees).

## 3. Architecture rules

**Backend (hexagonal).**
- `domain` holds types, invariants and pure functions. It imports nothing from `application`, `adapters`, `infrastructure`,
  `transport` or any I/O package.
- `application` holds use cases. It depends on **ports** (interfaces it owns), never on concrete adapters. One service per
  bounded context (`auth`, `posts`, `accounts`, `media`, `scheduler`, …). Authorization (scopes, ownership, verification) is
  checked here, not in handlers.
- `adapters`/`infrastructure` implement ports (Postgres, Redis/Asynq, S3, providers, mail). They contain no business rules.
- `transport` (HTTP, middleware) only decodes, calls one service method and encodes. No SQL, no provider calls, no decisions.
- Errors: return `errs` codes from the domain/application layer; map them to HTTP once in `httpx`. Never leak internal error
  text to clients; log it with context.
- Time comes from the injected clock; randomness from `crypto/rand`; configuration only from env via `config.Load` (validated,
  fail fast, warnings collected in `Config.Warnings`).
- **Tenancy**: every query on user data filters by `user_id` (or joins through an owned row). Every new resource gets a case
  in `internal/e2e/security_test.go` proving user B cannot read or change user A's data.
- **Idempotency and the post state machine** follow ARCHITECTURE; publishing happens only in the scheduler/publisher path,
  never before `scheduled_at`, and every attempt is recorded.
- **Migrations**: goose, numbered, reviewed SQL. Additive first; destructive changes are split across two releases (add →
  migrate data → drop later). Every new query on a large table states the index it uses.

**Providers (social networks).** One adapter per network behind the provider port, plus a fake for tests. Capabilities are
declared honestly (post types, limits, review requirements) and drive the UI and MCP; an unsupported feature returns
`ErrUnsupported`, never a fake success. Credentials are encrypted at rest; tokens never appear in logs, audit metadata,
errors or the frontend. Each adapter has an `httptest` fake-server test of the real request/response shapes, including auth
failure → "reconnect needed". New networks follow [PLATFORMS](docs/PLATFORMS.md).

**MCP server.** Tools are thin: validate input, call the REST API with the caller's key, map errors to helpful hints. No
business logic and no database access. Tool descriptions are part of the product: precise, with limits and side effects
stated. Destructive tools are marked and must be confirmed server-side (see D-007 and later decisions).

**Frontend.** Talk to the API only through `src/lib/api.ts`; no raw `fetch` in components. Every data view has explicit
loading, empty, error and success states. Server state lives in the API, not in `localStorage` (which is for UI preferences
only). Components are small and composed; shared UI goes into the shared components folder instead of being copied. The demo
mock (`src/lib/demo`, `scripts/mock-api.mjs`) must answer every new endpoint, so the Pages demo never breaks.

**Deploy.** Scripts are idempotent, `set -Eeuo pipefail`, shellcheck-clean and tested in `deploy/tests`. Anything that runs
as root on the production host touches only `/opt/socialos`, its systemd units and the single Caddy `import` line.

## 4. Size and shape

| | File | Function | Enforced by |
|---|---|---|---|
| Norm (all packages) | ≤ 250 lines | ≤ 60 lines | review |
| Ceiling | Go 400, TS/TSX 300 | 80 | linters in CI (ratchet: lowered over time, never raised) |

Test files are exempt from the file limit but are split by topic. Generated files, fixtures and the demo seed are listed as
known debt with a reason. Signs that a split is due even under the limit: the file needs "and" to be named; editing one
function means scrolling the file; imports from many layers at the top; two functions that differ in three lines.

## 5. Code quality

- Names say what, not how (`ScheduleForUser`, not `HandleData2`). No abbreviations a newcomer must guess.
- Comments explain **why** (a constraint, a trade-off, a link to a decision), not what the next line does.
- No dead code, commented-out code, TODO without an issue link, or "temporary" flags without an owner and removal date.
- No swallowed errors: no empty `catch`, no `_ = err` on anything that can fail meaningfully, no "failed → show empty list".
  Error ≠ empty. The user gets a clear message; the log gets the cause.
- Logs are structured (key-value), at the right level, and never contain secrets, tokens, passwords, full emails or post
  bodies of other users. Mask PII.
- No premature abstraction: a second use case justifies an interface, a third justifies a helper. Ports are the exception.
- New dependencies need a reason in the PR (what it replaces, size, maintenance, licence). Prefer the standard library.
- Concurrency: every goroutine has an owner and a stop path (context); no shared mutable state without a lock or a channel.
- Performance: no N+1 queries, paginate every list endpoint, bound every input (sizes, counts, lengths).
- Unrelated refactoring goes in its own PR. Behaviour changes are written down (PR description, CHANGELOG `Unreleased`).

## 6. Tests

- **Fast by default.** `make test` needs no network, Docker or database: Postgres/Redis/providers are replaced by fakes of
  the ports, the frontend by a mock of `lib/api.ts`. Integration and e2e tests live behind `make test-integration`.
- **What to test**: behaviour and contracts: state transitions, authorization (no scope → 403, other tenant → 404), validation,
  limits and quotas, idempotency, error mapping, PII masking, every UI state (loading/empty/error). Not implementation
  details: a test that breaks on a rename is a bad test.
- **A bug fix starts with a red test** that reproduces it; the PR shows it failed before the fix.
- Security-relevant code gets negative tests (forged headers, reused tokens, other tenant, oversized input).
- Flaky tests are fixed or quarantined with an issue the same day; never retried until green and forgotten.
- A test is never weakened or deleted to make CI pass.

## 7. Product rules

- **Honesty over polish.** If a network needs app review, a paid tier or is not supported, the UI, the API capabilities and
  the MCP tool descriptions say so. Never simulate success.
- **The human stays in control.** Agents act through scoped keys; dangerous actions (publish now, delete, disconnect)
  require explicit confirmation; every agent action is visible in the audit log.
- **Every screen has a next step.** Empty states explain what to do and offer the action. Errors say what happened and how to
  fix it (reconnect, verify email, raise a limit), in plain English, without stack traces or codes alone.
- **Mobile and accessibility are not optional**: works at 390 px without horizontal scroll, keyboard reachable, visible focus,
  labels on inputs, contrast AA.
- **Privacy by default**: collect the minimum, no third-party trackers, export and deletion are always available.
- UI copy, code, commits and docs are in English. Short sentences; one term per concept across UI, API and docs.
- UI text lives in `frontend/messages/en.json` (ICU, with a note for translators in `meta.json`), never in JSX or `lib` code;
  `npm run i18n:literals` fails the build on hard-coded strings. Counts and states are `plural` / `select` messages, never
  string concatenation. See [translation-process](docs/copy/translation-process.md).
- Design follows the repo's design language; new screens reuse existing components and tokens. Design skills
  (frontend-design, design-taste, web-design-guidelines, ui-ux-pro-max, …) inform choices but never override this file.

## 8. Security baseline (OWASP ASVS L1)

- No secrets in git, ever (code, tests, docs, examples use obvious placeholders). Production secrets live only in the
  server's `.env` (mode 600). GitHub holds no app secrets.
- Validate all input at the boundary; encode all output; parameterized SQL only.
- AuthN/AuthZ on every route; CSRF on cookie-authenticated mutations; rate limits on auth, mail and agent traffic, keyed by
  the client IP resolved through `TRUSTED_PROXIES` only.
- No server-side fetch of user-supplied URLs without an allow-list and SSRF guards; uploads are size- and type-checked.
- Changes to auth, crypto, tenancy, uploads, providers or deploy need a `security-reviewer` pass before merge.
- CI scanning (govulncheck, npm audit, gitleaks, Trivy, CodeQL) stays green; a finding is fixed or explicitly accepted in
  `docs/SECURITY.md` with a reason and a date.

## 9. Git, PRs and releases

- Branch per change (`feat/…`, `fix/…`, `docs/…`, `ci/…`), PR into `main`, squash merge. `main` is always releasable.
- Commits: Conventional Commits, English, no `Co-Authored-By` trailer. Author identity in this repo is the repo-local one
  (`Abzal Serikbay <89690044+XXX1694@users.noreply.github.com>`); never commit with another identity.
- PR description: what and why, how it was tested (commands + results), the acceptance scenario output, screenshots for UI,
  risks and follow-ups. Link the decision or issue.
- Merge only with green CI and a review; sensitive areas (section 8) also need the security review.
- Releases: update `CHANGELOG.md`, tag `vX.Y.Z` on `main` (tags are protected; only admins create them). Images build, the
  GitHub Release is created from the CHANGELOG section, and servers with autoupdate deploy it within minutes.
- Nothing is published outside the repository (social posts, npm, MCP registries, announcements) without the owner's "yes".

## 10. Review checklist

- [ ] Does it do what the plan and the acceptance scenario say, including the unhappy paths?
- [ ] Layering respected (section 3); no business logic in transport, adapters or MCP tools.
- [ ] Tenancy: every new query scoped; a security test exists for new resources.
- [ ] Errors: mapped once, user-facing text clear, causes logged, nothing swallowed.
- [ ] No secrets or PII in logs, audit metadata, errors or the frontend.
- [ ] Tests cover behaviour, including negative cases; a bug fix has a red-then-green test.
- [ ] Sizes within the norms; no duplicate implementation; old code removed.
- [ ] Contract changes reflected in ARCHITECTURE, MCP tool descriptions, the demo mock and the CHANGELOG.
- [ ] UI: every state handled, mobile 390 px checked, screenshots attached and looked at.

## 11. Definition of done

- [ ] `make lint`, `make test` and the relevant integration/deploy tests are green locally and in CI.
- [ ] The acceptance scenario was run by hand; its output is in the PR.
- [ ] New behaviour has tests; bug fixes have a regression test.
- [ ] Docs updated where behaviour changed (ARCHITECTURE, DECISIONS, README, CHANGELOG `Unreleased`).
- [ ] Review (and security review where required) passed; findings fixed or filed as issues.
- [ ] For deploy changes: verified on the server and the other site on the host still answers.

## 12. Never

- Commit secrets; print secrets in logs or chat; put secrets in GitHub.
- Touch anything on the production host outside `/opt/socialos`, its systemd units and the one Caddy `import` line; another
  project runs there.
- Run `git stash`, force-push `main`, rewrite published history of `main`, or force-push a branch without a backup branch and
  `--force-with-lease`.
- Fake a capability, a success or a test result.
- Weaken a test or lower a quality ceiling to get CI green.
- Spend money, publish anything, or change DNS without the owner's explicit approval.
