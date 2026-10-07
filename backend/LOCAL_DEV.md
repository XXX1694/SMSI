# Local development

State of the sandbox this was built in (all on 127.0.0.1, left running):

| Service | Where | How it was started |
|---|---|---|
| PostgreSQL 16 | `127.0.0.1:5432`, role `socialos` / password `socialos` (superuser), DBs `socialos` (dev, migrated) and `socialos_test` | `su postgres -c "/usr/lib/postgresql/16/bin/pg_ctl -D /home/claude/pgdata -l /home/claude/pglogs/pg.log -o '-p 5432 -c listen_addresses=127.0.0.1 -c unix_socket_directories=/tmp -c fsync=off -c max_connections=200' start"` |
| Redis | `127.0.0.1:6379` (DB 0 for dev, DB 15 for tests), no persistence | `redis-server --bind 127.0.0.1 --port 6379 --dir /home/claude/redisdata --save "" --daemonize yes` |

Check them: `pg_isready -h 127.0.0.1 -p 5432` and `redis-cli ping`.
No Docker daemon and no MinIO are available here, so object storage is the in-memory driver (see the caveat below).

## Environment variables

Tests (exported by `make test-integration`, shown here for running `go test` by hand):

```bash
export TEST_DATABASE_URL='postgres://socialos:socialos@127.0.0.1:5432/socialos_test?sslmode=disable'
export TEST_REDIS_URL='redis://127.0.0.1:6379/15'
```

`TEST_DATABASE_URL` is only the admin connection: every test creates and migrates its own throw-away database `socialos_it_*` and drops it afterwards. Without these variables the integration tests **skip** (they never fail), so `go test ./...` is always safe.

Running the API and worker: `make env` writes `backend/.env` (git-ignored) from `.env.example` with a fresh `ENCRYPTION_KEY`. The `.env` in this sandbox is already created with:

```
APP_ENV=development
DATABASE_URL=postgres://socialos:socialos@127.0.0.1:5432/socialos?sslmode=disable
REDIS_URL=redis://127.0.0.1:6379/0
ENCRYPTION_KEY=<openssl rand -base64 32>      # keep it: it encrypts stored OAuth tokens
SOCIAL_MOCK_PROVIDERS=true
STORAGE_DRIVER=memory
API_PUBLIC_URL=http://localhost:8080  WEB_BASE_URL=http://localhost:3000  CORS_ALLOWED_ORIGINS=http://localhost:3000
```

Without `make`, export the same variables yourself, e.g.
`set -a; . ./.env; set +a`.

## Commands (run in `/home/claude/SMSI/backend`)

```bash
make build              # bin/api bin/worker bin/migrate (static, CGO off)
make test               # unit tests only, no services needed
make test-integration   # everything against real Postgres + Redis (about 1 minute)
make lint               # gofmt, go vet, golangci-lint
make migrate            # apply migrations to DATABASE_URL from .env
make run-api            # API on :8080   (foreground)
make run-worker         # worker + reconciler, health on :8081 (foreground)
```

Run API and worker in the background and stop them again:

```bash
setsid nohup make run-api    > /tmp/socialos-api.log    2>&1 &
setsid nohup make run-worker > /tmp/socialos-worker.log 2>&1 &
# stop (graceful):
pkill -TERM -f 'exe/api'; pkill -TERM -f 'exe/worker'; pkill -TERM -f 'go run ./cmd/'
```

## Smoke test with curl

```bash
curl -s localhost:8080/health                       # {"status":"ok"}
curl -s -i localhost:8080/ready                     # 200 when postgres, redis and storage are ok; 503 + "errors" otherwise

J=$(mktemp); curl -s -c $J -X POST localhost:8080/api/v1/auth/register -H 'Content-Type: application/json' \
  -d '{"email":"me@example.com","password":"correct horse battery","display_name":"Me"}'
curl -s -b $J localhost:8080/api/v1/me              # {"user":{...},"scopes":[...],"csrf_token":"...","auth_type":"session","api_key":null,...}
CSRF=$(curl -s -b $J localhost:8080/api/v1/me | python3 -c 'import sys,json;print(json.load(sys.stdin)["csrf_token"])')

# connect the mock network (the browser flow, followed by hand)
L=$(curl -s -b $J -c $J -o /dev/null -w '%{redirect_url}' 'localhost:8080/api/v1/social/mock/connect?redirect=/accounts')
curl -s -b $J -c $J -o /dev/null -w '%{redirect_url}\n' "$L"      # -> http://localhost:3000/accounts?connected=mock

# an agent key that may draft and schedule but not publish
KEY=$(curl -s -b $J -X POST localhost:8080/api/v1/developer/api-keys -H "X-CSRF-Token: $CSRF" -H 'Content-Type: application/json' \
  -d '{"name":"agent","scopes":["social:read","posts:read","posts:write","posts:schedule"]}' | python3 -c 'import sys,json;print(json.load(sys.stdin)["key"])')
ACC=$(curl -s -H "Authorization: Bearer $KEY" localhost:8080/api/v1/social/accounts | python3 -c 'import sys,json;print(json.load(sys.stdin)["items"][0]["id"])')
PID=$(curl -s -X POST localhost:8080/api/v1/posts -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' \
  -d "{\"content\":\"hello\",\"social_account_ids\":[\"$ACC\"]}" | python3 -c 'import sys,json;print(json.load(sys.stdin)["id"])')
curl -s -X POST localhost:8080/api/v1/posts/$PID/publish -H "Authorization: Bearer $KEY"     # 403 INSUFFICIENT_SCOPE (posts:publish)
curl -s -X POST localhost:8080/api/v1/posts/$PID/schedule -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' \
  -d "{\"scheduled_at\":\"$(date -u -d '+5 seconds' +%Y-%m-%dT%H:%M:%SZ)\"}"
sleep 8; curl -s -H "Authorization: Bearer $KEY" localhost:8080/api/v1/posts/$PID/status     # "published" (worker must be running)
curl -s -b $J 'localhost:8080/api/v1/audit-logs?limit=10'                                   # who did what
```

Handy mock-network controls in post text: `#mock-fail` (permanent error), `#mock-retry` (503 once, then ok), `#mock-auth` (token revoked, account expires), `#mock-unknown` (recorded but the call times out).

## Caveats

- **Storage**: `STORAGE_DRIVER=memory` lives inside one process. The API can store and serve uploads, but the **worker cannot read them**. The mock network never reads media bytes, so mock posts with images still publish; Telegram and LinkedIn would fail to find the file. For real media publishing use S3-compatible storage, e.g. MinIO on a machine with Docker:
  `docker run -d -p 9000:9000 -e MINIO_ROOT_USER=minio -e MINIO_ROOT_PASSWORD=change-me-please minio/minio server /data`
  and set `STORAGE_DRIVER=s3`, `S3_ENDPOINT=localhost:9000`, `S3_ACCESS_KEY=minio`, `S3_SECRET_KEY=change-me-please` (the bucket is created automatically).
- **S3 unreachable** is not fatal: the API starts, logs a warning and `/ready` answers 503 with `{"checks":{"storage":"unavailable"},"errors":{"storage":"..."}}`. Uploads return 500 until it is back.
- **Reset the dev database**: `psql 'postgres://socialos:socialos@127.0.0.1:5432/postgres' -c 'DROP DATABASE socialos WITH (FORCE)' -c 'CREATE DATABASE socialos' && make migrate`, and `redis-cli -n 0 flushdb` for queued jobs. Postgres is the source of truth for schedules: after a Redis flush the reconciler re-enqueues overdue jobs within a minute.
- **Container image**: `make docker` (not buildable in this sandbox). Run migrations with `docker run --env-file .env socialos-backend /app/migrate up`.
- Never reuse the sandbox `ENCRYPTION_KEY` or the `socialos`/`socialos` database password anywhere else.
