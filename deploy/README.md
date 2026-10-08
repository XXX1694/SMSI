# Deploying SocialOS on one server

A single VPS runs the whole product with Docker Compose. Caddy terminates HTTPS (Let's Encrypt, automatic) and is the only
thing that listens on the internet; everything else lives on the internal compose network.

There are two ways to run it. **Standalone** (this page, sections 1-12) is the default: the stack brings its own Caddy and owns
ports 80/443. **Host-proxy mode** (section 13) is for a server where a reverse proxy already owns 80/443: the stack then listens
on `127.0.0.1` only and that proxy forwards to it. Updates can be pushed from GitHub (section 8) or pulled by the server from
GitHub Releases (section 14).

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
| `docker-compose.host-proxy.yml` | override for host-proxy mode: no bundled Caddy, ports on `127.0.0.1`, memory limits for a small host (section 13) |
| `host-proxy/` | `render-caddy.sh` stages the host Caddy snippet from `.env`; `install-caddy-import.sh` swaps it in, adds the one import line, validates, reloads and rolls everything back on failure (section 13) |
| `autoupdate.sh`, `systemd/` | pull-based updates: a timer deploys the latest GitHub Release (section 14) |
| `tests/` | shell tests for the scripts above (`bash tests/run.sh`, needs Linux; no Docker daemon, no network) |
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

Full bot setup (admin rights, link codes, limits): [docs/integrations/telegram.md](../docs/integrations/telegram.md). Run `webhook-info` before `set-webhook` so you do not take over a bot that another service uses.

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

Full walkthrough (Page verification, products, token lifetime, troubleshooting): [docs/integrations/linkedin.md](../docs/integrations/linkedin.md).

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

Prefer to keep every secret off GitHub, or the server cannot accept SSH from the internet? Let the server pull instead: section 14.

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

## Monitoring

The `Uptime` workflow (`.github/workflows/uptime.yml`) probes the public endpoints every 15 minutes and needs no external
account. Each URL gets 3 attempts with backoff and a 10 s timeout; 2xx and 3xx count as up. It also warns when a TLS
certificate expires in under 14 days.

- **Outage**: an issue "Uptime: <url> is down" with the label `incident` is opened. Further failures comment on it instead
  of opening duplicates. When the URL answers again, the workflow comments "recovered after <duration>" and closes it.
  Watch the repository (Issues) to get the notification by email.
- **Targets**: by default `${API_PUBLIC_URL}/ready`, the app login (`API_PUBLIC_URL` with `api.` replaced by `app.`, plus
  `/login`) and `${MCP_PUBLIC_URL%/mcp}/health`, all from repository variables (section 3). To watch other URLs set the
  repository variable `UPTIME_URLS` to a space-separated list; it replaces the defaults. With neither variable set the run
  is skipped with a notice.
- **Delays**: GitHub may delay or drop scheduled runs under load, so 15 minutes is a target, not a guarantee. Run the
  workflow by hand from the Actions tab (`workflow_dispatch`) for an immediate check.

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
| `install-caddy-import.sh` exits 1 and says ROLLED BACK | the message shows the validator output or the URL that stopped answering; fix it (a hostname already defined in the host's Caddyfile is the usual cause of "ambiguous site definition") and run it again. The staged file is still in `caddy/staging/` |
| `install-caddy-import.sh` exits 4 (ROLLBACK NOT VERIFIED) | read its last messages: either a file could not be restored (the command to copy it back is printed), or Caddy could not be reloaded, or your sites still do not answer: `systemctl status caddy`, `journalctl -u caddy -n 50` |
| `install-caddy-import.sh` exits 2 | a prerequisite is missing and nothing was changed: not root, nothing staged (run `render-caddy.sh`), no `HOST_PROXY_CHECK_URLS`, or `/opt/socialos/caddy` is not root-owned or is writable by others |
| `install-caddy-import.sh` exits 3 | one of `HOST_PROXY_CHECK_URLS` was already failing before the change, so nothing was touched: fix that site first |
| the timer deploys nothing | `journalctl -u socialos-autoupdate -n 30`, `./autoupdate.sh --dry-run` (it says "not newer", "not a release version", "skipping", ...), `systemctl list-timers`, `.deploy/autoupdate_failed` (section 14) |

## 13. Behind an existing reverse proxy (host-proxy mode)

Use this when the server already runs a web server that owns ports 80 and 443 (here: **Caddy running as a systemd service**, not in
Docker, with its configuration in `/etc/caddy/Caddyfile`). SocialOS then lives in `/opt/socialos`, runs in Docker, publishes
its ports on `127.0.0.1` only, and the host Caddy serves `app.`, `api.`, `mcp.` and `s3.<domain>` by importing one generated file.

```
 browser / agents ─► host Caddy :80/:443 ─┬─ app.<domain> ─► 127.0.0.1:13000 frontend ─► backend ─► postgres / redis
 (its other sites are untouched)          ├─ api.<domain> ─► 127.0.0.1:18080 backend
                                          ├─ mcp.<domain> ─► 127.0.0.1:13333 mcp
                                          └─ s3.<domain>  ─► 127.0.0.1:19000 minio   (only with the bundled MinIO)
```

**What the setup changes on the host, and nothing else:** the Docker packages; the directory `/opt/socialos`; one
`import /opt/socialos/caddy/*.caddy` line in `/etc/caddy/Caddyfile` (after a timestamped backup of that file, and only after
`caddy validate` and a check of your other sites); optionally two systemd unit files (section 14). No global Caddy option, no
firewall rule and no other service is touched. The generated Caddy file is only *staged* until the installer swaps it in, and
any failure puts the previous state back (13.5).

| In host-proxy mode | Standalone | Host-proxy |
|---|---|---|
| Bundled Caddy | runs on 80/443 | not started |
| Published ports | Caddy only | `127.0.0.1:13000` frontend, `:18080` backend, `:13333` mcp, `:19000` minio (`FRONTEND_HOST_PORT`, `BACKEND_HOST_PORT`, `MCP_HOST_PORT`, `MINIO_HOST_PORT`) |
| Memory limit (hard cap, override with `<SERVICE>_MEM_LIMIT`) | postgres 768m, redis 192m, minio 512m, backend 384m, worker 384m, mcp 192m, frontend 512m, caddy 192m | postgres 256m, redis 64m, minio 160m, backend 160m, worker 160m, mcp 96m, frontend 160m: about 1 GB of caps, about 0.3 GB in use when idle |
| PostgreSQL | image defaults | `shared_buffers=64MB`, `max_connections=40`, `work_mem=4MB` (`POSTGRES_SHARED_BUFFERS`, `POSTGRES_MAX_CONNECTIONS`, `POSTGRES_WORK_MEM`), pools of 10 (`DB_MAX_CONNS`) |

All variables are documented in `.env.prod.example`. A container above its cap is OOM-killed and restarted by Docker; it never
takes memory from the host's other service. Every SocialOS container also has `oom_score_adj: 500`, so if the *host* ever runs
out of memory the kernel picks these containers before the host's Caddy or the other service, and Postgres gets a 64 MB
`/dev/shm` (`POSTGRES_SHM_SIZE`, in step with `shared_buffers`). Watch `docker stats --no-stream` and `free -m` during the first days, and raise a
limit when `docker inspect -f '{{.State.OOMKilled}}' <container>` says `true`.

### 13.1 DNS

Section 1 applies unchanged: `app.`, `api.`, `mcp.` (and `s3.` with the bundled MinIO) must resolve to the server before the
host Caddy can get certificates. With no domain yet, `<ip-with-dashes>.sslip.io` (the server's IPv4 address with dots written as dashes) resolves to
that IP and works for a trial; its certificates share Let's Encrypt rate limits with everyone else who uses it, so move to a real
domain for anything lasting.

### 13.2 Install Docker (official apt repository, Ubuntu 24.04)

Skip this if `docker compose version` already works. As root:

```bash
apt-get update && apt-get install -y ca-certificates curl
install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
chmod a+r /etc/apt/keyrings/docker.asc
cat >/etc/apt/sources.list.d/docker.sources <<EOF
Types: deb
URIs: https://download.docker.com/linux/ubuntu
Suites: $(. /etc/os-release && echo "${UBUNTU_CODENAME:-$VERSION_CODENAME}")
Components: stable
Signed-By: /etc/apt/keyrings/docker.asc
EOF
apt-get update
apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
docker compose version
```

Docker adds its own firewall rules (`iptables`) and sets the `FORWARD` policy to `DROP`; check that nothing else on the host
depends on forwarding. The SocialOS ports are bound to `127.0.0.1`, so `ufw` and the cloud firewall need no change. If the images
are private, log in once as root (section 2); public packages need nothing.

### 13.3 Put the files in `/opt/socialos`

The server needs the `deploy/` folder of the release you are installing (the compose files and scripts are versioned with the
images). As root, with `vX.Y.Z` a release tag and `<owner>/<repo>` the GitHub repository:

```bash
install -d -m 755 /opt/socialos && cd /opt/socialos
curl -fsSL "https://codeload.github.com/<owner>/<repo>/tar.gz/refs/tags/vX.Y.Z" |
  tar -xz --strip-components=2 --wildcards '*/deploy/*'
ls -A                                   # docker-compose.prod.yml, docker-compose.host-proxy.yml, deploy.sh, host-proxy/, ...
```

(`scp -r deploy/. root@<server>:/opt/socialos/` from a checkout does the same.) Everything stays owned by root; `.env` is mode
600 and holds every secret. GitHub holds none of them.

**`/opt/socialos` and `/opt/socialos/caddy` must stay root-owned, mode 755: never `chown` them to a deploy user.** Whatever is in
`/opt/socialos/caddy` becomes configuration of the host's Caddy, which also serves the other site; anyone who can write there
could change what that Caddy does. `install-caddy-import.sh` refuses to run when these folders (or the files in them) belong to
someone else or are writable by group or others.

### 13.4 Create `.env`

```bash
cd /opt/socialos
./init-env.sh --host-proxy example.com   # .env (mode 600) with generated secrets and the COMPOSE_FILE below
grep ^COMPOSE_FILE .env                  # COMPOSE_FILE=docker-compose.prod.yml:docker-compose.host-proxy.yml
$EDITOR .env
```

In the editor: set `HOST_PROXY_CHECK_URLS` (next step), add `LINKEDIN_*` and `TELEGRAM_BOT_TOKEN` if you use them, and change a
`*_HOST_PORT` if something on the host already listens on it (check with `ss -ltn`). `ACME_EMAIL` stays empty: only the bundled
Caddy uses it, and the host's Caddy handles certificates (an e-mail given as a second argument is stored, but unused).
Back up `.env` (section 4). `docker compose config -q` must print nothing.

### 13.5 Connect the host Caddy

```bash
./host-proxy/render-caddy.sh             # reads DOMAIN, S3_SITE and the ports from .env, STAGES /opt/socialos/caddy/staging/socialos.caddy
less caddy/staging/socialos.caddy        # concrete hostnames, same headers/routes as deploy/Caddyfile
HOST_PROXY_CHECK_URLS="https://www.example.org/" ./host-proxy/install-caddy-import.sh   # as root
```

Rendering changes nothing the host's Caddy can see: `caddy/staging/` is not matched by the import line. Only the installer moves
the staged file into `/opt/socialos/caddy`. `HOST_PROXY_CHECK_URLS` is a space-separated list of sites the host already serves (put
it in `.env` to avoid typing it); every one must answer with a status below 500 **before** the change (otherwise the script stops,
says "Nothing was changed" and touches nothing) and **after** it. The installer:

1. checks that the folders are root-owned and not writable by others, and that something is staged;
2. copies `/etc/caddy/Caddyfile` to `/etc/caddy/Caddyfile.socialos-backup-<UTC time>`;
3. copies the staged snippet into `/opt/socialos/caddy` (the live set becomes exactly the staged set) and appends
   `import /opt/socialos/caddy/*.caddy` unless the line is already there;
4. runs `caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile` (as the `caddy` user when it exists);
5. runs `systemctl reload caddy`, then re-checks your URLs (a few tries);
6. if any step fails, or the script is interrupted, it restores the Caddyfile from the backup and the snippet set that Caddy last
   loaded successfully (files that are not in that set are deleted), reloads Caddy, checks the URLs again and prints which state
   the host is in: which files were restored, whether Caddy runs the restored configuration (or never loaded the new one) and
   whether your sites answer. A rollback that could not be completed or verified exits 4 and says **ROLLBACK NOT VERIFIED**; act on
   its messages first.

Exit codes: 0 done; 1 failed and rolled back, verified; 2 could not start (nothing changed); 3 a check URL was already failing
(nothing changed); 4 failed, rollback not verified. It is safe to run again, for example after changing a port or the domain and
running `render-caddy.sh` again. The snippet names its helper `(socialos_common)` and sets no global option, so it cannot collide
with the host's own snippets or `email`/ACME settings. Undo by hand: `cp -p /etc/caddy/Caddyfile.socialos-backup-<time>
/etc/caddy/Caddyfile`, delete `/opt/socialos/caddy/*.caddy` and `/opt/socialos/caddy/.applied`, then `systemctl reload caddy`.

The validator runs in your shell, not inside the Caddy service. If the host Caddyfile uses `{$VARIABLES}` that the caddy unit
defines (an `Environment=` or `EnvironmentFile=` line), export the same variables before running the script, or validation fails
and the script rolls back (safe, but it changes nothing).

### 13.6 First deploy

```bash
cd /opt/socialos
./deploy.sh 1.2.3                        # image tag: the release tag without the "v" (or sha-<7 hex>, section 4)
./deploy.sh --status                     # tags, the compose files in use, container state
docker stats --no-stream; free -m
curl -fsS https://api.example.com/ready; curl -fsSI https://app.example.com/login | head -1
```

`deploy.sh` uses the `COMPOSE_FILE` of `.env`, so it starts the stack without Caddy, waits for `/ready` inside the network, and
then checks `https://api.<domain>/ready` and `https://app.<domain>/login` through the host Caddy (informational: a certificate that
is still being issued does not roll the images back). The plain `docker compose ...` commands of sections 5, 6, 9 and 10 work as
written.

### 13.7 Telegram webhook

With `TELEGRAM_BOT_TOKEN` set and the stack running:

```bash
docker compose run --rm telegram webhook-info     # first: does another consumer already use this bot?
docker compose run --rm telegram set-webhook
```

### 13.8 Updates

Either push deploys (section 8) or the pull-based timer in section 14. After a release that changes `deploy/` (the release notes
say so), refresh the files as in 13.3 (`.env`, `.deploy/` and `caddy/` are not part of the archive, so they stay), run
`./host-proxy/render-caddy.sh` and `./host-proxy/install-caddy-import.sh` again (it swaps in the staged file only if it is valid).

## 14. Pull-based updates

The server polls GitHub and deploys by itself; nothing can reach the server from outside and GitHub needs no secret. A systemd
timer runs `autoupdate.sh` every 5 minutes (plus up to a minute of random delay; a run missed while the machine was off happens at
boot). Each run:

1. asks `https://api.github.com/repos/<repo>/releases/latest` (public API, no token; `GITHUB_REPO` in `.env`, default `XXX1694/SMSI`);
2. accepts only a tag shaped `vX.Y.Z` (pre-releases and drafts are never "latest"; anything else is logged and ignored);
3. compares it with the tag `deploy.sh` recorded in `.deploy/current_tag` (the image tag is the release tag without the `v`, as
   `release.yml` names the images) and goes on only for a **strictly newer** version, compared as numbers (`1.10.0` is newer than
   `1.9.0`). An older or equal release is refused and logged, so the timer never downgrades; a deployed tag that is not a
   release version (`sha-...`, `main`) is left alone until you deploy a release by hand once (`./deploy.sh X.Y.Z`), after which
   the timer takes over;
4. checks anonymously, the way a `docker pull` of a public package does (token plus a HEAD on the manifest), that
   `ghcr.io/<owner>/socialos-{backend,mcp,frontend}:X.Y.Z` all exist. If not, it logs "images not ready", records nothing and
   looks again at the next run (a release can show up before its images; the Release workflow now publishes the GitHub
   Release only after the images, so this is a safety net);
5. runs `./deploy.sh X.Y.Z`: pull, migrate, restart, wait for `/ready`, roll back on failure. `flock` keeps two runs from
   overlapping.

**Trust boundary.** Whoever can publish a `vX.Y.Z` release in the repository decides what runs in production: within about 5
minutes the server pulls that release's images, runs their database migrations and starts them, with no further approval. The
server trusts GitHub and the repository's release process, nothing else. So keep write access to the repository minimal, protect
the `v*` tags (a GitHub *tag ruleset* that restricts who can create, update or delete them), and treat a compromised maintainer
account or release token as a compromised production. The "strictly newer" rule limits the damage of a mistaken or replayed old
tag; it does not make a malicious newer release safe. If you need a human in the loop, use `deploy.yml` with required reviewers
(section 8) instead, or set `AUTOUPDATE=false` and deploy by hand.

```bash
# as root, once
cd /opt/socialos
install -m 644 systemd/socialos-autoupdate.service systemd/socialos-autoupdate.timer /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now socialos-autoupdate.timer
./autoupdate.sh --dry-run                # what would happen right now
systemctl list-timers socialos-autoupdate.timer
journalctl -u socialos-autoupdate -n 50  # one short line per run, plus deploy.sh's output when it deploys
```

| Situation | What happens |
|---|---|
| `AUTOUPDATE=false` in `.env` (also `0`, `no`, `off`) | the timer keeps running and every run logs "disabled" and does nothing |
| the release exists but its images are not there yet | "images not ready" is logged, nothing is recorded (no failure marker, no delay), the next run (about 5 minutes) looks again. Private packages cannot be seen anonymously: `AUTOUPDATE_CHECK_IMAGES=false` skips the check |
| the check passes but `deploy.sh` cannot pull anyway | `deploy.sh` changes nothing and exits 75; the next try is 15 minutes later |
| ghcr.io cannot be reached or answers with an error | "could not check ghcr.io" is logged, nothing is deployed or recorded, the next run tries again |
| the deploy fails and is rolled back | `.deploy/autoupdate_failed` records the version and that run exits non-zero (see `systemctl --failed` and the journal). The version is not tried again: later runs only log "skipping", until a newer release appears or you run `rm .deploy/autoupdate_failed` |
| GitHub is unreachable, rate-limited (60 requests/hour per IP without a token; this uses 12) or has no release yet | logged, nothing deployed, the unit stays green |
| the latest release is older than or equal to the deployed one | refused and logged ("not newer"), nothing is deployed, nothing is recorded as a failure |
| the deployed tag is `sha-...` or `main` (a hand deploy) | the timer does nothing and says why, until you run `./deploy.sh X.Y.Z` for a release once |
| you rolled back by hand (`./deploy.sh --rollback`) | the latest release is newer than the rolled-back tag, so the next run deploys it again: set `AUTOUPDATE=false` first (`deploy.sh` reminds you) |

It updates the three SocialOS images only. The files in `/opt/socialos` (compose files, scripts, `.env`) are never touched; refresh
them when a release changes `deploy/` (13.8). Because migrations stay backward compatible (section 5), a rollback to the previous
tag works against the migrated database.

**Pull or push?** Prefer the pull timer when the secrets must stay off GitHub, the server cannot be reached over SSH from the
internet, or you want production to follow tagged releases only. Prefer `deploy.yml` (section 8) when you want every merge to
`main` (`sha-` tags) deployed, a required reviewer approval in GitHub in front of each deployment, an immediate rollout instead of
one within about 5 minutes, or the deploy output in the Actions log. The two can be combined, but not both on the same tag stream
without thought: after a push deploy of a `sha-` tag the timer stays idle until a release is deployed by hand, and after a push
deploy of an older release it would deploy the latest one.

To run the script tests: `bash tests/run.sh` from `deploy/` on Linux (on macOS: `docker run --rm -v "$PWD:/repo" -w /repo
ubuntu:24.04 bash deploy/tests/run.sh` from the repository root).

## 15. Releasing

A release is a git tag `vX.Y.Z` on `main`. Everything after the tag is automatic.

1. **Update `CHANGELOG.md`** in a pull request: move the entries from `## [Unreleased]` into a new `## [X.Y.Z] - YYYY-MM-DD`
   section (Added / Changed / Fixed / Security, written for users), and add the link lines at the bottom (`[X.Y.Z]: .../releases/tag/vX.Y.Z`,
   and point `[Unreleased]` at `compare/vX.Y.Z...HEAD`). Merge it once CI is green. The Release workflow fails if the section for the
   tag is missing or empty.
2. **Tag `main`** at the merge commit: `git tag vX.Y.Z <sha> && git push origin vX.Y.Z`. The `v*` tags are protected, so only an
   administrator can create them. A pre-release candidate is `vX.Y.Z-rc.N`; it is published as a GitHub pre-release and is never
   "latest", so servers do not pick it up.
3. **The images build.** `release.yml` pushes `ghcr.io/<owner>/socialos-{backend,mcp,frontend}` with the tags `X.Y.Z`, `X.Y` and
   (from 1.0) `X`. Set the repository variables `API_PUBLIC_URL` and `MCP_PUBLIC_URL` first (section 3, *Frontend URLs*): they are baked
   into the frontend image.
4. **The GitHub Release is created automatically**, as the last job and only if all three images were pushed. Its notes are the
   tag's `CHANGELOG.md` section. Re-running the workflow only refreshes the notes of an existing release.
5. **Servers with pull-based updates (section 14) deploy it** within about 5 minutes: `releases/latest` now names the tag, the images
   exist, the version is newer than the deployed one. Servers without the timer deploy it by hand (`./deploy.sh X.Y.Z`) or through
   `deploy.yml` (section 8).

If the Release workflow fails on the CHANGELOG check, fix the changelog on `main`, then move the tag to the fixed commit
(`git tag -f vX.Y.Z <sha> && git push -f origin vX.Y.Z`; an administrator can do that despite the protection) or delete the tag and
push it again. No server has seen the release yet, because it does not exist before its images and notes do.
