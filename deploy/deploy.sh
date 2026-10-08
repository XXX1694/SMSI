#!/usr/bin/env bash
# Deploy a published SocialOS image tag on this server (single-host docker compose).
#
#   ./deploy.sh <tag>               pull <tag>, run migrations, restart, wait for /ready, roll back on failure
#   ./deploy.sh <tag> --no-migrate  same, without running migrations
#   ./deploy.sh --rollback          redeploy the previously deployed tag (never runs migrations)
#   ./deploy.sh --status            show the deployed tags and the container state
#
# <tag> is an image tag in GHCR: sha-<7 hex> (every merge to main), main, or X.Y.Z (the git tag vX.Y.Z without the "v").
# Used by .github/workflows/deploy.yml (over SSH), by autoupdate.sh (systemd timer) and fine to run by hand.
# Needs ./.env (see .env.prod.example). Exit status 75 means "nothing was changed, try again later" (the images could not
# be pulled, or another deploy holds the lock); 1 is a failed deploy.
set -Eeuo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"

STATE_DIR=.deploy
APP_SERVICES=(backend worker migrate mcp frontend)
READY_TIMEOUT=${READY_TIMEOUT:-120}
SKIP_PUBLIC_CHECK=${SKIP_PUBLIC_CHECK:-0} # 1 = do not probe https://api.$DOMAIN and https://app.$DOMAIN after the deploy
STACK_TOUCHED=false # becomes true once the running containers may have been changed (i.e. after a successful pull)

log() { printf '==> %s\n' "$*"; }
warn() { printf '!!  %s\n' "$*" >&2; }
die() { warn "$*"; exit 1; }
usage() { sed -n '2,12p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; }

# Which compose files make up the stack: COMPOSE_FILE from the environment or from .env (as `docker compose` itself reads
# it), default docker-compose.prod.yml. Host-proxy mode adds docker-compose.host-proxy.yml there.
dc() { docker compose "$@"; }
# has_service NAME: is the service part of the stack in this mode? (the bundled caddy is not in host-proxy mode)
has_service() {
  local services
  services=$(dc config --services 2>/dev/null) || return 1
  grep -qx "$1" <<<"$services"
}

# env_get KEY: last assignment of KEY in .env, without surrounding quotes.
env_get() {
  local line
  line=$(grep -E "^[[:space:]]*$1=" .env | tail -n 1 || true)
  line=${line#*=}
  line=${line%\"}
  line=${line#\"}
  line=${line%\'}
  line=${line#\'}
  printf '%s' "$line"
}

# set_env_tag TAG: make .env point at TAG so plain `docker compose` commands use it as well.
set_env_tag() {
  if grep -q '^IMAGE_TAG=' .env; then
    sed -i "s|^IMAGE_TAG=.*|IMAGE_TAG=$1|" .env
  else
    printf '\nIMAGE_TAG=%s\n' "$1" >>.env
  fi
}

read_state() { cat "$STATE_DIR/$1" 2>/dev/null || true; }

wait_ready() {
  local deadline=$((SECONDS + READY_TIMEOUT))
  log "waiting for the API to report ready (GET /ready: Postgres, Redis, storage), up to ${READY_TIMEOUT}s"
  # The distroless image has no shell or curl; the binary doubles as the probe and exits non-zero unless HTTP 200.
  until dc exec -T backend /app/api healthcheck http://127.0.0.1:8080/ready >/dev/null 2>&1; do
    if ((SECONDS >= deadline)); then
      warn "the API did not become ready within ${READY_TIMEOUT}s"
      return 1
    fi
    sleep 3
  done
  log "the API is ready"
}

# Informational only: a pending certificate or DNS record is not fixed by rolling the images back.
public_check() {
  local domain url try ok proxy_hint
  domain=$(env_get DOMAIN)
  if has_service caddy; then
    proxy_hint="docker compose logs caddy"
  else
    proxy_hint="host-proxy mode: is the SocialOS import in the host's Caddyfile (host-proxy/install-caddy-import.sh)? journalctl -u caddy"
  fi
  if [ "$SKIP_PUBLIC_CHECK" = 1 ] || [ -z "$domain" ] || ! command -v curl >/dev/null 2>&1; then return 0; fi
  for url in "https://api.${domain}/ready" "https://app.${domain}/login"; do
    ok=false
    for try in $(seq 1 12); do
      if curl -fsS -o /dev/null --max-time 10 "$url" 2>/dev/null; then
        ok=true
        break
      fi
      if [ "$try" -lt 12 ]; then sleep 5; fi
    done
    if $ok; then
      log "reachable over HTTPS: $url"
    else
      warn "NOT reachable over HTTPS: $url (DNS record, firewall or certificate still pending? $proxy_hint)"
    fi
  done
}

# cleanup_images KEEP_TAG...: remove older socialos images (the registry still has them, a rollback re-pulls).
cleanup_images() {
  local owner ref keep skip
  owner=$(env_get GHCR_OWNER)
  owner=${owner:-xxx1694}
  while read -r ref; do
    skip=false
    for keep in "$@"; do
      if [ -n "$keep" ] && [ "${ref##*:}" = "$keep" ]; then skip=true; fi
    done
    if [ "$skip" = false ]; then docker image rm "$ref" >/dev/null 2>&1 || true; fi
  done < <(docker image ls --format '{{.Repository}}:{{.Tag}}' | grep -E "^ghcr\.io/${owner}/socialos-(backend|mcp|frontend):" || true)
}

# Untagged leftovers of the SocialOS images only (a moving tag such as main leaves them behind). Never a bare
# `docker image prune`: on a shared host that would also delete other workloads' untagged images. release.yml labels the
# images org.opencontainers.image.title=socialos-<name>.
prune_socialos_images() {
  local name
  for name in backend mcp frontend; do
    docker image prune -f --filter "label=org.opencontainers.image.title=socialos-$name" >/dev/null 2>&1 || true
  done
}

diagnose() {
  local services=(migrate backend worker mcp frontend)
  if has_service caddy; then services+=(caddy); fi
  warn "deploying $1 failed. Container state and recent logs:"
  dc ps -a || true
  dc logs --no-color --tail=60 "${services[@]}" 2>&1 || true
}

# deploy TAG SKIP_MIGRATE. Every step ends in `|| return 1`: errexit is disabled inside `if deploy ...`.
deploy() {
  local new=$1 skip_migrate=$2
  export IMAGE_TAG=$new # beats .env for every compose call below; .env is updated once the deploy has succeeded

  log "pulling the application images for $new"
  if ! dc pull "${APP_SERVICES[@]}"; then
    warn "pull failed. Does the tag exist (GitHub > Packages), and is this server logged in to ghcr.io (docker login ghcr.io)?"
    return 1
  fi
  STACK_TOUCHED=true

  log "starting PostgreSQL and Redis (unchanged containers are left alone)"
  dc up -d --wait --wait-timeout 120 postgres redis || return 1

  if [ "$skip_migrate" = false ]; then
    log "applying database migrations"
    dc run --rm -T migrate || return 1
  fi

  log "starting the stack"
  dc up -d --remove-orphans --wait --wait-timeout 240 || return 1
  wait_ready || return 1
}

mode=deploy
tag=""
skip_migrate=false
while [ $# -gt 0 ]; do
  case "$1" in
    --rollback) mode=rollback ;;
    --status) mode=status ;;
    --no-migrate) skip_migrate=true ;;
    -h | --help)
      usage
      exit 0
      ;;
    -*)
      usage
      die "unknown option: $1"
      ;;
    *) tag=$1 ;;
  esac
  shift
done

command -v docker >/dev/null 2>&1 || die "docker is not installed"
docker compose version >/dev/null 2>&1 || die "the docker compose plugin is not installed"
[ -f .env ] || die ".env is missing. Create it first: ./init-env.sh <domain> <acme-email>"
COMPOSE_FILE=${COMPOSE_FILE:-$(env_get COMPOSE_FILE)}
export COMPOSE_FILE=${COMPOSE_FILE:-docker-compose.prod.yml}
mkdir -p "$STATE_DIR"

current=$(read_state current_tag)
previous=$(read_state previous_tag)

if [ "$mode" = status ]; then
  echo "deployed tag : ${current:-none}"
  echo "previous tag : ${previous:-none}"
  echo "IMAGE_TAG    : $(env_get IMAGE_TAG)"
  echo "compose files: $COMPOSE_FILE"
  if [ -f "$STATE_DIR/history.log" ]; then
    echo "history:"
    tail -n 10 "$STATE_DIR/history.log"
  fi
  dc ps
  exit 0
fi

if [ "$mode" = rollback ]; then
  [ -n "$previous" ] || die "no previous tag recorded in $STATE_DIR/previous_tag; pass one explicitly: ./deploy.sh sha-<7 hex>"
  tag=$previous
  skip_migrate=true
fi

[ -n "$tag" ] || {
  usage
  die "missing image tag"
}
[[ "$tag" =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$ ]] || die "invalid image tag: $tag"
# ACME_EMAIL belongs to the bundled Caddy only: without that service (host-proxy mode) it may be empty or a placeholder.
placeholders=$(grep -nE '^[A-Za-z_][A-Za-z0-9_]*=.*CHANGE_ME' .env || true)
if ! has_service caddy; then placeholders=$(grep -vE '^[0-9]+:ACME_EMAIL=' <<<"$placeholders" || true); fi
if [ -n "$placeholders" ]; then
  while IFS= read -r line; do printf '%s=...\n' "${line%%=*}" >&2; done <<<"$placeholders"
  die ".env still contains CHANGE_ME placeholders (see the variables above)"
fi
dc config -q || die "the compose files ($COMPOSE_FILE) do not resolve with the current .env (see the error above)"
if has_service caddy && [ -z "${ACME_EMAIL:-$(env_get ACME_EMAIL)}" ]; then
  die "ACME_EMAIL is empty in .env, and the bundled Caddy needs it. (Behind a reverse proxy that owns ports 80/443 use host-proxy mode instead: README section 13.)"
fi

command -v flock >/dev/null 2>&1 || die "flock is required (util-linux)"
exec 9>"$STATE_DIR/lock"
flock -n 9 || {
  warn "another deploy is already running"
  exit 75
}

log "deploying $tag (currently: ${current:-nothing deployed by deploy.sh yet})"
if deploy "$tag" "$skip_migrate"; then
  set_env_tag "$tag"
  if [ "$current" != "$tag" ]; then
    [ -z "$current" ] || printf '%s\n' "$current" >"$STATE_DIR/previous_tag"
    printf '%s\n' "$tag" >"$STATE_DIR/current_tag"
  fi
  printf '%s %s ok\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$tag" >>"$STATE_DIR/history.log"
  cleanup_images "$tag" "$(read_state previous_tag)"
  prune_socialos_images
  public_check
  log "DEPLOYED $tag (previous: $(read_state previous_tag)). Roll back with: ./deploy.sh --rollback"
  if [ "$mode" = rollback ]; then
    warn "If the autoupdate timer is installed, set AUTOUPDATE=false in .env now: it deploys the latest release again, which is newer than $tag."
  fi
  dc ps
else
  printf '%s %s FAILED\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$tag" >>"$STATE_DIR/history.log"
  diagnose "$tag"
  if [ "$STACK_TOUCHED" = false ]; then
    warn "nothing was changed: ${current:-the current stack} keeps running. Fix the problem and run ./deploy.sh $tag again"
    exit 75
  elif [ -n "$current" ] && [ "$current" != "$tag" ]; then
    warn "rolling back to $current"
    export IMAGE_TAG=$current
    if dc up -d --remove-orphans --wait --wait-timeout 240 && wait_ready; then
      warn "rolled back: $current is running again. Fix the problem, then deploy again."
    else
      warn "THE ROLLBACK FAILED TOO. Check: docker compose ps; docker compose logs (in $(pwd))"
    fi
  else
    warn "no earlier deployment to roll back to; fix the problem and run ./deploy.sh $tag again"
  fi
  exit 1
fi
