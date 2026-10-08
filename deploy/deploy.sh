#!/usr/bin/env bash
# Deploy a published SocialOS image tag on this server (single-host docker compose).
#
#   ./deploy.sh <tag>               pull <tag>, run migrations, restart, wait for /ready, roll back on failure
#   ./deploy.sh <tag> --no-migrate  same, without running migrations
#   ./deploy.sh --rollback          redeploy the previously deployed tag (never runs migrations)
#   ./deploy.sh --status            show the deployed tags and the container state
#
# <tag> is an image tag in GHCR: sha-<7 hex> (every merge to main), main, or vX.Y.Z.
# Used by .github/workflows/deploy.yml (over SSH) and fine to run by hand. Needs ./.env (see .env.prod.example).
set -Eeuo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"

COMPOSE_YML=docker-compose.prod.yml
STATE_DIR=.deploy
APP_SERVICES=(backend worker migrate mcp frontend)
READY_TIMEOUT=${READY_TIMEOUT:-120}
SKIP_PUBLIC_CHECK=${SKIP_PUBLIC_CHECK:-0} # 1 = do not probe https://api.$DOMAIN and https://app.$DOMAIN after the deploy
STACK_TOUCHED=false # becomes true once the running containers may have been changed (i.e. after a successful pull)

log() { printf '==> %s\n' "$*"; }
warn() { printf '!!  %s\n' "$*" >&2; }
die() { warn "$*"; exit 1; }
usage() { sed -n '2,10p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; }

dc() { docker compose -f "$COMPOSE_YML" "$@"; }

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
  local domain url try ok
  domain=$(env_get DOMAIN)
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
      warn "NOT reachable over HTTPS: $url (DNS record, firewall or certificate still pending? docker compose -f $COMPOSE_YML logs caddy)"
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

diagnose() {
  warn "deploying $1 failed. Container state and recent logs:"
  dc ps -a || true
  dc logs --no-color --tail=60 migrate backend worker mcp frontend caddy 2>&1 || true
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
mkdir -p "$STATE_DIR"

current=$(read_state current_tag)
previous=$(read_state previous_tag)

if [ "$mode" = status ]; then
  echo "deployed tag : ${current:-none}"
  echo "previous tag : ${previous:-none}"
  echo "IMAGE_TAG    : $(env_get IMAGE_TAG)"
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
if grep -qE '^[A-Za-z_][A-Za-z0-9_]*=.*CHANGE_ME' .env; then
  grep -nE '^[A-Za-z_][A-Za-z0-9_]*=.*CHANGE_ME' .env | sed 's/=.*/=.../' >&2
  die ".env still contains CHANGE_ME placeholders (see the variables above)"
fi
dc config -q || die "docker-compose.prod.yml does not resolve with the current .env (see the error above)"

command -v flock >/dev/null 2>&1 || die "flock is required (util-linux)"
exec 9>"$STATE_DIR/lock"
flock -n 9 || die "another deploy is already running"

log "deploying $tag (currently: ${current:-nothing deployed by deploy.sh yet})"
if deploy "$tag" "$skip_migrate"; then
  set_env_tag "$tag"
  if [ "$current" != "$tag" ]; then
    [ -z "$current" ] || printf '%s\n' "$current" >"$STATE_DIR/previous_tag"
    printf '%s\n' "$tag" >"$STATE_DIR/current_tag"
  fi
  printf '%s %s ok\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$tag" >>"$STATE_DIR/history.log"
  cleanup_images "$tag" "$(read_state previous_tag)"
  docker image prune -f >/dev/null 2>&1 || true
  public_check
  log "DEPLOYED $tag (previous: $(read_state previous_tag)). Roll back with: ./deploy.sh --rollback"
  dc ps
else
  printf '%s %s FAILED\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$tag" >>"$STATE_DIR/history.log"
  diagnose "$tag"
  if [ "$STACK_TOUCHED" = false ]; then
    warn "nothing was changed: ${current:-the current stack} keeps running. Fix the problem and run ./deploy.sh $tag again"
  elif [ -n "$current" ] && [ "$current" != "$tag" ]; then
    warn "rolling back to $current"
    export IMAGE_TAG=$current
    if dc up -d --remove-orphans --wait --wait-timeout 240 && wait_ready; then
      warn "rolled back: $current is running again. Fix the problem, then deploy again."
    else
      warn "THE ROLLBACK FAILED TOO. Check: docker compose -f $COMPOSE_YML ps; docker compose -f $COMPOSE_YML logs"
    fi
  else
    warn "no earlier deployment to roll back to; fix the problem and run ./deploy.sh $tag again"
  fi
  exit 1
fi
