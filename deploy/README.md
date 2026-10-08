# Deploying SocialOS on one server

A single VPS runs the whole product with Docker Compose. Caddy terminates HTTPS (Let's Encrypt, automatic) and is the only
thing that listens on the internet; everything else lives on the internal compose network.

```
 browser ──► app.example.com  ─┐
 agents  ──► mcp.example.com   ─┤  Caddy :80/:443 ─► frontend :3000 ──(/api/v1/*)──► backend :8080 ─► postgres
 clients ──► api.example.com  ─┤                                                        │      └──► redis ◄── worker
 browser ──► s3.example.com   ─┘ (presigned media URLs)  ─► minio :9000                 └─────────► minio (or S3 / R2)
```

| Name | Served by | Used for |
|---|---|---|
| `app.<domain>` | frontend | the web UI; `/api/v1/*` is proxied to the backend, so browser cookies are first-party |
| `api.<domain>` | backend | API-key clients, **OAuth callbacks (LinkedIn)**, **Telegram webhook**, `/ready` |
| `mcp.<domain>` | mcp | MCP endpoint `https://mcp.<domain>/mcp` for AI agents |
| `s3.<domain>` | minio | presigned media URLs (only with the bundled MinIO) |

| File | Purpose |
|---|---|
| `docker-compose.prod.yml` | the stack: GHCR images, Postgres, Redis, MinIO, Caddy. No database port is published |
| `Caddyfile` | hostnames, HTTPS, headers; `/metrics` is not routed publicly |
| `.env.prod.example` | every variable, documented. Copy to `.env` (or use `init-env.sh`) |
| `init-env.sh` | creates `.env` with freshly generated secrets (`openssl rand`) |
| `deploy.sh` | pull an image tag, migrate, restart, wait for `/ready`, roll back automatically on failure |
| `backup.sh` | `pg_dump` (+ optional media archive) with retention, for cron |

Images are built by `.github/workflows/release.yml` and published as `ghcr.io/<owner>/socialos-{backend,mcp,frontend}`.
Tags: `sha-<7 hex>` (every merge to `main`, immutable, what you deploy), `main` (moves), `X.Y.Z` / `X.Y` / `X` for `vX.Y.Z` git tags.

## 1. DNS records

Create these records at your DNS provider **before the first deploy**, all pointing at the server's public IP (add `AAAA` records
for IPv6 if the server has an address). Caddy cannot get certificates until they resolve.

```
app.example.com.   A   203.0.113.10
api.example.com.   A   203.0.113.10
mcp.example.com.   A   203.0.113.10
s3.example.com.    A   203.0.113.10      # only with the bundled MinIO
```

If the domain is on Cloudflare, use **DNS only** (grey cloud) until HTTPS works, or set SSL/TLS to *Full (strict)* and keep the
proxy off for `mcp` (Streamable HTTP) and `s3` (large uploads).

## 2. Prepare the server

Any Linux VPS with 2 vCPU / 2-4 GB RAM and Docker Engine 24+ with the Compose plugin (Ubuntu 22.04/24.04 shown).

```bash
# as root
curl -fsSL https://get.docker.com | sh
adduser --disabled-password --gecos "" deploy
usermod -aG docker deploy                      # the deploy user can run docker; treat it as root-equivalent
mkdir -p /opt/socialos && chown deploy: /opt/socialos

ufw allow OpenSSH && ufw allow 80/tcp && ufw allow 443/tcp && ufw allow 443/udp && ufw --force enable
```

Only Caddy publishes ports (80, 443). Note that Docker publishes ports around `ufw`, which is exactly why Postgres, Redis, MinIO
and the app containers publish none.

**Let the server pull the (private) images.** Create a GitHub *personal access token (classic)* with the single scope
`read:packages` and log in once as the `deploy` user (the login is stored in `~/.docker/config.json`):

```bash
echo "<token>" | docker login ghcr.io -u <your-github-username> --password-stdin
```

Alternative: make the three packages public (GitHub > your profile > Packages > each package > *Package settings* > *Change
visibility*). The images contain no secrets (all secrets are runtime variables), and public packages are not subject to the
private-package storage/transfer quota of your plan, which a stream of `sha-` tags can exhaust on the Free plan.

## 3. Configure GitHub (before the first release)

*Settings > Secrets and variables > Actions > Variables* (not secrets):

| Variable | Value | Why |
|---|---|---|
| `API_PUBLIC_URL` | `https://api.example.com` | baked into the frontend image |
| `MCP_PUBLIC_URL` | `https://mcp.example.com/mcp` | baked into the frontend image |
| `API_INTERNAL_URL` | *(leave unset)* default `http://backend:8080` | the backend's name inside the compose network |
| `RELEASE_PLATFORMS` | *(optional)* e.g. `linux/amd64` | default is `linux/amd64,linux/arm64` |

Then push to `main` (or push a tag `v0.1.0`). CI runs, and when it succeeds the **Release** workflow builds and pushes the images.
Check *Packages* on the repository page for `socialos-backend`, `socialos-mcp`, `socialos-frontend`.
The `workflow_run` trigger only exists once the workflow files are on the default branch.

### Frontend URLs (read this once)

The frontend image bakes in `API_INTERNAL_URL` (the destination of the `/api/v1/*` rewrite in `next.config.mjs`) and
`NEXT_PUBLIC_API_URL` / `NEXT_PUBLIC_MCP_URL` (shown in the generated MCP client configs). They are fixed when the image is built,
so **they cannot be changed in `.env` on the server**. The release workflow takes them from the repository variables above.

- Built before the variables were set? The release prints a warning and the UI shows `localhost` URLs. Set the variables and run
  *Actions > Release > Run workflow* on `main`, then deploy the new `sha-` tag.
- Changed your domain? Same: update the variables, re-run Release, deploy.
- `API_INTERNAL_URL` must match the compose service name of the backend (`backend`). Do not rename the service without changing it.

## 4. First deploy

From your machine, copy the `deploy/` folder to the server and create the configuration:

```bash
scp -r deploy/. deploy@203.0.113.10:/opt/socialos/
ssh deploy@203.0.113.10
cd /opt/socialos
./init-env.sh example.com you@example.com    # writes .env (mode 600) with generated secrets
$EDITOR .env                                 # optional: LINKEDIN_*, TELEGRAM_BOT_TOKEN, external S3 ...
```

`.env.prod.example` documents every variable and the `openssl rand` command behind each secret. **Back up `.env`** (password manager,
not the server only): without `ENCRYPTION_KEY` all stored OAuth tokens become unreadable.

Pick the image tag of the release you want (GitHub > Packages, or the *Release* run summary) and deploy it:

```bash
./deploy.sh sha-1a2b3c4
```

`deploy.sh` pulls the three images, starts Postgres and Redis, **runs the migrations**, starts the stack, waits until
`GET /ready` (Postgres, Redis and storage) answers 200 inside the network and then checks `https://api.<domain>/ready` and
`https://app.<domain>/login` from the outside. If something fails it prints the container state and logs and puts the previous
tag back. Open `https://app.example.com`, register, and you are in.

Day to day: `docker compose ps`, `docker compose logs -f backend worker`, `./deploy.sh --status` (the `.env` sets `COMPOSE_FILE`,
so plain `docker compose` works in this directory).

## 5. Migrations

Migrations are SQL files embedded in the backend image (goose). They run automatically:

- `./deploy.sh <tag>` runs `migrate up` with the new image **before** the app containers are replaced (skip with `--no-migrate`);
- the `migrate` service is also a dependency of `backend` and `worker`, so a plain `docker compose up -d` applies them too.

By hand:

```bash
docker compose run --rm -T migrate                       # = /app/migrate up
docker compose run --rm -T migrate /app/migrate status   # which migrations are applied
docker compose run --rm -T migrate /app/migrate down     # revert the LAST migration (data loss possible: back up first)
```

Keep migrations backward compatible with the previous release (add columns/tables first, remove them one release later). A
rollback restarts the old images **against the already migrated database**; it does not undo migrations.

## 6. Telegram webhook

Production runs `TELEGRAM_UPDATES_MODE=webhook` (the worker does not long-poll). With `TELEGRAM_BOT_TOKEN` and
`TELEGRAM_WEBHOOK_SECRET` set in `.env` and the stack running, register the webhook once (this is `make -C backend
telegram-set-webhook` for the production stack):

```bash
docker compose run --rm telegram set-webhook         # registers https://api.<domain>/api/v1/webhooks/telegram with the secret
docker compose run --rm telegram webhook-info        # url, pending updates, last error reported by Telegram
docker compose run --rm telegram delete-webhook      # remove it (needed before switching to polling)
```

Run `set-webhook` again after changing the domain or `TELEGRAM_WEBHOOK_SECRET` (and `docker compose up -d` first, so that the
API containers pick up the new value). Telegram only delivers to https on port 443/8443 with a valid certificate: check `webhook-info`
if link codes are not picked up.

## 7. LinkedIn

In your app at <https://www.linkedin.com/developers/apps>: *Products*: **Sign In with LinkedIn using OpenID Connect** and **Share on
LinkedIn**; *Auth* > *Authorized redirect URLs for your app*:

```
https://api.example.com/api/v1/social/linkedin/callback
```

(`${API_PUBLIC_URL}/api/v1/social/linkedin/callback`.) Put `LINKEDIN_CLIENT_ID` and `LINKEDIN_CLIENT_SECRET` in `.env`, then
`docker compose up -d` to recreate the containers.

The user logs in on `app.<domain>` but LinkedIn sends the browser back to `api.<domain>`. The session cookie therefore has to be
valid on both, which is what `COOKIE_DOMAIN` (default: `$DOMAIN`) does. Do not clear it: without it the callback arrives without a
session and the connection fails with `UNAUTHENTICATED`.

## 8. Automatic deploys from GitHub

`ci.yml` tests every pull request and every push to `main`. After a green `main`, `release.yml` pushes images and then starts
`deploy.yml` for exactly that commit. The deploy only runs when these **repository secrets** exist (otherwise it is skipped with
a notice):

| Secret | Value |
|---|---|
| `DEPLOY_HOST` | server name or IP |
| `DEPLOY_USER` | `deploy` |
| `DEPLOY_SSH_KEY` | private key, see below |
| `DEPLOY_KNOWN_HOSTS` | *(recommended)* `ssh-keyscan -t ed25519 <host>` output, pins the host key |

Optional variables: `DEPLOY_PATH` (default `/opt/socialos`), `DEPLOY_PORT` (default `22`).

```bash
ssh-keygen -t ed25519 -N "" -C "github-actions-deploy" -f ./socialos_deploy     # on your machine
ssh-copy-id -i ./socialos_deploy.pub deploy@203.0.113.10                        # public half -> server
# paste the content of ./socialos_deploy into the DEPLOY_SSH_KEY secret, then delete both files
```

The job uses the **`production` environment**: in *Settings > Environments > production* add *Required reviewers* to approve every
deployment (and, if you like, limit it to the `main` branch). The workflow uploads this folder (never `.env`), runs
`./deploy.sh <tag>` and shows the result. You can also run it by hand: *Actions > Deploy > Run workflow*, with a tag or empty for
the latest `main`. The first deploy still needs `.env` on the server (section 4).

## 9. Backups

What matters: the Postgres database, `.env` (above all `ENCRYPTION_KEY`), and the media (MinIO volume, or your S3 bucket, which
you back up on the provider's side).

```bash
./backup.sh                     # /var/backups/socialos/socialos-db-<UTC time>.dump (pg_dump custom format), 14 days kept
BACKUP_MEDIA=1 ./backup.sh      # also socialos-media-<time>.tar.gz of the bundled MinIO volume
```

Cron example (`crontab -e` as the deploy user; `BACKUP_DIR` / `KEEP_DAYS` can be set in the line):

```cron
17 3 * * *  /opt/socialos/backup.sh >>/var/log/socialos-backup.log 2>&1
```

Copy `/var/backups/socialos` off the machine (rsync/rclone/restic to another provider); a backup on the same disk is not a
backup. Restore into a fresh or emptied stack:

```bash
docker compose stop backend worker mcp frontend
docker compose exec -T postgres sh -c 'dropdb -U "$POSTGRES_USER" --if-exists "$POSTGRES_DB" && createdb -U "$POSTGRES_USER" "$POSTGRES_DB"'
docker compose exec -T postgres sh -c 'pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --no-owner' < /var/backups/socialos/socialos-db-<time>.dump
docker compose up -d
```

Test a restore once on a spare machine; an untested backup is a hope.

## 10. Rollback

Every successful deploy records the tag it replaced.

```bash
./deploy.sh --status             # deployed tag, previous tag, last deployments
./deploy.sh --rollback           # redeploy the previous tag (no migrations); run it again to switch back
./deploy.sh sha-9f8e7d6          # or any tag that still exists in GHCR (use --no-migrate to leave the schema alone)
```

The same works from GitHub: *Actions > Deploy > Run workflow* with an older tag. A failed deploy rolls itself back to the
previous tag; a failed *migration* leaves the old containers running (the new ones never start). Migrations are not reverted:
see section 5.

## 11. Operations notes

- **Logs**: `docker compose logs -f --tail=100 backend worker caddy`. Container logs rotate (5 x 10 MB each).
- **MinIO console** (bucket browser) is not published. To look inside, temporarily add `ports: ["127.0.0.1:9001:9001"]` to the
  `minio` service, `docker compose up -d minio`, open it through `ssh -L 9001:localhost:9001 deploy@<host>`, and remove the line again.
- **External S3 / Cloudflare R2** instead of MinIO: see the block in `.env.prod.example` (set `COMPOSE_PROFILES=` empty, `S3_ENDPOINT`,
  `S3_USE_SSL=true`, `S3_PUBLIC_ENDPOINT=` empty, `S3_SITE=:8099`), then `docker compose up -d --remove-orphans`. Presigned URLs then
  point at your provider, so `s3.<domain>` and its DNS record are not needed.
- **Update Postgres/Redis/Caddy/MinIO**: `deploy.sh` only pulls the SocialOS images, so infrastructure images change when you decide:
  `docker compose pull postgres redis caddy && docker compose up -d`. A Postgres **major** upgrade needs dump and restore (section 9).
- **Certificates** are in the `caddy-data` volume and renew automatically. While testing DNS, uncomment the staging CA in the
  `Caddyfile` to avoid Let's Encrypt rate limits.
- **Free-plan minutes** (2000 per month for private repositories, billed per job and rounded up to the minute): pull requests only
  run the jobs whose paths changed and a newer push cancels the run it replaces; a push to `main` runs everything (an estimate is
  20-30 minutes with warm caches) and Release adds a few more. Look at *Settings > Billing* after the first week and adjust.
  Dependabot opens a handful of grouped pull requests per week, and each one runs CI.

## 12. Troubleshooting

| Symptom | Look at |
|---|---|
| `deploy.sh`: pull denied / manifest unknown | the tag exists under Packages; `docker login ghcr.io` was done as the `deploy` user |
| Caddy: `no certificate` / ACME errors | `docker compose logs caddy`; DNS records resolve to this server; ports 80 and 443 reachable from outside |
| `deploy.sh` waits and then fails at `/ready` | `docker compose logs backend`: `/ready` needs Postgres, Redis **and storage**; with the bundled MinIO check `docker compose ps minio` |
| backend exits at start with `config: ...` | the message names the variable: `COOKIE_SECURE`, `ENCRYPTION_KEY` (base64 of 32 bytes), webhook secret (16+ chars of `A-Za-z0-9_-`) |
| UI shows `localhost` URLs in the MCP config | the frontend image was built without the repo variables (section 3, *Frontend URLs*) |
| LinkedIn: `?error=UNAUTHENTICATED` after consent | `COOKIE_DOMAIN` was cleared or the redirect URL points at another host than `API_PUBLIC_URL` (section 7) |
| images of media do not load in the UI | `s3.<domain>` DNS record and certificate; the browser must reach `https://s3.<domain>` |
