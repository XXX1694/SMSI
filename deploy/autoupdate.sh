#!/usr/bin/env bash
# Pull-based update: deploy the latest GitHub Release of the repository when it differs from what is running.
# Started every few minutes by the systemd timer (systemd/socialos-autoupdate.timer); also fine to run by hand.
#
#   ./autoupdate.sh              check the latest release and deploy it if it is new
#   ./autoupdate.sh --dry-run    only say what would happen
#
# Reads from .env: AUTOUPDATE (false/0/no/off = do nothing), GITHUB_REPO (default XXX1694/SMSI).
# Asks https://api.github.com/repos/<repo>/releases/latest (no token). Only a tag shaped vX.Y.Z is accepted; the image tag
# is the same without the "v" (that is how release.yml names it), and the deployed tag is what deploy.sh recorded in
# .deploy/current_tag. A new tag is handed to ./deploy.sh, which pulls, migrates, waits for /ready and rolls back on failure.
# A tag whose deploy failed is remembered in .deploy/autoupdate_failed and not tried again until a newer release appears
# (delete that file to retry; a manual `./deploy.sh <tag>` that succeeds makes it irrelevant). Output goes to the journal:
#   journalctl -u socialos-autoupdate
# It updates the three SocialOS images only. The files in this directory (compose files, scripts) are not touched.
set -Eeuo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"

STATE_DIR=.deploy
RETRY_DELAY=${AUTOUPDATE_RETRY_DELAY:-900} # seconds to wait after "nothing was changed" (images not published yet, ...)

log() { printf 'autoupdate: %s\n' "$*"; }
warn() { printf 'autoupdate: %s\n' "$*" >&2; }
usage() { sed -n '2,15p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; }

dry_run=false
while [ $# -gt 0 ]; do
  case "$1" in
    --dry-run) dry_run=true ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      usage >&2
      exit 2
      ;;
  esac
  shift
done

[ -f .env ] || {
  warn ".env is missing in $(pwd)"
  exit 1
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
read_state() { cat "$STATE_DIR/$1" 2>/dev/null || true; }

enabled=$(env_get AUTOUPDATE)
case "$(printf '%s' "$enabled" | tr '[:upper:]' '[:lower:]')" in
  false | 0 | no | off)
    log "disabled (AUTOUPDATE=$enabled in .env)"
    exit 0
    ;;
esac

repo=$(env_get GITHUB_REPO)
repo=${repo:-XXX1694/SMSI}
[[ "$repo" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || {
  warn "GITHUB_REPO in .env is not <owner>/<repo>: $repo"
  exit 1
}

command -v curl >/dev/null 2>&1 || {
  warn "curl is required"
  exit 1
}
command -v flock >/dev/null 2>&1 || {
  warn "flock is required (util-linux)"
  exit 1
}

mkdir -p "$STATE_DIR"
exec 8>"$STATE_DIR/autoupdate.lock"
flock -n 8 || {
  log "the previous run is still working, skipping"
  exit 0
}

# ---- what is the latest release? -------------------------------------------------------------------------------------
body=$(mktemp)
trap 'rm -f "$body"' EXIT
code=$(curl -sS --max-time 30 -o "$body" -w '%{http_code}' \
  -H 'Accept: application/vnd.github+json' -H 'X-GitHub-Api-Version: 2022-11-28' -H 'User-Agent: socialos-autoupdate' \
  "https://api.github.com/repos/$repo/releases/latest" 2>/dev/null || true)
case "$code" in
  200) ;;
  404)
    log "no published release in $repo yet"
    exit 0
    ;;
  *)
    # Network trouble and rate limits are not a failure of this host: try again at the next tick.
    warn "could not read the latest release of $repo (HTTP ${code:-000}); will try again"
    exit 0
    ;;
esac

tag=""
tag_re='"tag_name"[[:space:]]*:[[:space:]]*"([^"]*)"'
if [[ "$(<"$body")" =~ $tag_re ]]; then tag=${BASH_REMATCH[1]}; fi
if ! [[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  warn "ignoring the latest release of $repo: '${tag:-?}' is not a vX.Y.Z tag"
  exit 0
fi
version=${tag#v}

# ---- is it new, and has it been tried? ----------------------------------------------------------------------------------
current=$(read_state current_tag)
if [ "$current" = "$version" ] || [ "$current" = "$tag" ]; then
  log "up to date: $current"
  exit 0
fi

failed=$(read_state autoupdate_failed)
if [ "${failed%% *}" = "$version" ]; then
  log "skipping $tag: its deploy failed earlier (${failed#* }); remove $STATE_DIR/autoupdate_failed to try again"
  exit 0
fi

retry=$(read_state autoupdate_retry)
if [ "${retry%% *}" = "$version" ] && [ "$(date +%s)" -lt "${retry#* }" ]; then
  log "waiting to retry $tag (nothing was changed last time)"
  exit 0
fi

if [ "$dry_run" = true ]; then
  log "would deploy $tag as image tag $version (running: ${current:-nothing recorded})"
  exit 0
fi

# ---- deploy ----------------------------------------------------------------------------------------------------------
log "deploying $tag as image tag $version (running: ${current:-nothing recorded})"
rc=0
./deploy.sh "$version" 8>&- || rc=$?
case "$rc" in
  0)
    rm -f "$STATE_DIR/autoupdate_failed" "$STATE_DIR/autoupdate_retry"
    log "deployed $tag"
    ;;
  75)
    # deploy.sh changed nothing: the images are not in the registry yet, or a manual deploy holds the lock.
    printf '%s %s\n' "$version" "$(($(date +%s) + RETRY_DELAY))" >"$STATE_DIR/autoupdate_retry"
    log "$tag was not deployed and nothing was changed; will retry in $((RETRY_DELAY / 60)) min"
    ;;
  *)
    printf '%s %s\n' "$version" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >"$STATE_DIR/autoupdate_failed"
    warn "deploying $tag failed (exit $rc); deploy.sh rolled back to ${current:-the previous state} where it could. Not trying $tag again."
    exit 1
    ;;
esac
