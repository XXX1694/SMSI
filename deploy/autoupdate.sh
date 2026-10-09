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
# .deploy/current_tag. Only a STRICTLY NEWER version is deployed: an older or equal release is refused and logged, and when
# the deployed tag is not a release version (sha-..., main) nothing is done until a release was deployed by hand once.
# Before it deploys, it asks ghcr.io anonymously (token + HEAD on the manifest, like a docker pull) whether the three images
# exist for that version. A release can be published before its images are built: then it logs "images not ready", records
# nothing and tries again at the next tick. (AUTOUPDATE_CHECK_IMAGES=false skips the check, e.g. for private packages.)
# A new tag is handed to ./deploy.sh, which pulls, migrates, waits for /ready and rolls back on failure.
# A tag whose deploy failed is remembered in .deploy/autoupdate_failed and not tried again until a newer release appears
# (delete that file to retry; a manual `./deploy.sh <tag>` that succeeds makes it irrelevant). Output goes to the journal:
#   journalctl -u socialos-autoupdate
# It updates the three Steerpost images only. The files in this directory (compose files, scripts) are not touched.
# While .deploy/guard/shed exists (the host guard shed load because the host is under pressure) it deploys nothing.
set -Eeuo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"

STATE_DIR=.deploy
RETRY_DELAY=${AUTOUPDATE_RETRY_DELAY:-900} # seconds to wait after "nothing was changed" (images not published yet, ...)

log() { printf 'autoupdate: %s\n' "$*"; }
warn() { printf 'autoupdate: %s\n' "$*" >&2; }
usage() { sed -n '2,21p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; }

# image_state NAME VERSION: is ghcr.io/<owner>/socialos-NAME:VERSION there? Anonymously, as `docker pull` of a public package
# does it. Prints ready, missing (no such tag), denied (private or unknown package: not visible without a login) or error.
image_state() {
  local name=$1 version=$2 tok_body tok_code token code
  tok_body=$(mktemp)
  tok_code=$(curl -sS --max-time 20 -o "$tok_body" -w '%{http_code}' \
    "https://ghcr.io/token?service=ghcr.io&scope=repository:$ghcr_owner/socialos-$name:pull" 2>/dev/null || true)
  token=""
  token_re='"token"[[:space:]]*:[[:space:]]*"([^"]*)"'
  if [ "$tok_code" = 200 ] && [[ "$(<"$tok_body")" =~ $token_re ]]; then token=${BASH_REMATCH[1]}; fi
  rm -f "$tok_body"
  case "$tok_code" in
    200) ;;
    401 | 403)
      echo denied
      return 0
      ;;
    *)
      echo error
      return 0
      ;;
  esac
  [ -n "$token" ] || {
    echo error
    return 0
  }
  code=$(curl -sS -I -o /dev/null --max-time 20 -w '%{http_code}' -H "Authorization: Bearer $token" \
    -H 'Accept: application/vnd.oci.image.index.v1+json, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.docker.distribution.manifest.v2+json' \
    "https://ghcr.io/v2/$ghcr_owner/socialos-$name/manifests/$version" 2>/dev/null || true)
  case "$code" in
    200) echo ready ;;
    404) echo missing ;;
    401 | 403) echo denied ;;
    *) echo error ;;
  esac
}

# version_gt A B: is release version A strictly newer than B? Both are MAJOR.MINOR.PATCH (digits only). Compared field by
# field as numbers without arithmetic, so a long number cannot overflow, and 1.10.0 is newer than 1.9.0.
version_gt() {
  local -a a b
  local i x y
  IFS=. read -r -a a <<<"$1"
  IFS=. read -r -a b <<<"$2"
  for i in 0 1 2; do
    x=${a[i]}
    y=${b[i]}
    while [ "${#x}" -gt 1 ] && [ "${x:0:1}" = 0 ]; do x=${x:1}; done
    while [ "${#y}" -gt 1 ] && [ "${y:0:1}" = 0 ]; do y=${y:1}; done
    if [ "${#x}" -ne "${#y}" ]; then [ "${#x}" -gt "${#y}" ]; return; fi
    if [[ "$x" > "$y" ]]; then return 0; fi
    if [[ "$x" < "$y" ]]; then return 1; fi
  done
  return 1
}

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

# A deploy pulls and unpacks images: the heaviest thing Steerpost does to a host. Not while the guard is shedding load
# because the host is under pressure (host-proxy/socialos-guard.sh); it resumes by itself or with --resume.
if [ -e "$STATE_DIR/guard/shed" ]; then
  log "skipping: the host guard has shed Steerpost load (level $(cat "$STATE_DIR/guard/shed" 2>/dev/null || true)); see host-proxy/socialos-guard.sh --status"
  exit 0
fi

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

# Never a downgrade, never a sidestep: whoever can publish a release decides what runs here, so the one thing the timer
# must not do is move production backwards or sideways because of a tag that is older, equal or odd.
if [ -n "$current" ]; then
  deployed=${current#v}
  if ! [[ "$deployed" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    warn "not deploying $tag: the deployed tag '$current' is not a release version, so 'newer' is undefined. Deploy a release by hand once (./deploy.sh $version); after that the timer takes over"
    exit 0
  fi
  if ! version_gt "$version" "$deployed"; then
    warn "refusing $tag: it is not newer than the deployed $current. The timer never downgrades; to go back on purpose, run ./deploy.sh <tag> by hand"
    exit 0
  fi
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

# ---- are the images there? --------------------------------------------------------------------------------------------
# Not a failure and not recorded anywhere: the release is just ahead of its images. The next tick looks again.
ghcr_owner=$(env_get GHCR_OWNER)
ghcr_owner=$(printf '%s' "${ghcr_owner:-xxx1694}" | tr '[:upper:]' '[:lower:]')
case "$(printf '%s' "$(env_get AUTOUPDATE_CHECK_IMAGES)" | tr '[:upper:]' '[:lower:]')" in
  false | 0 | no | off) ;;
  *)
    not_ready=""
    unknown=""
    denied=""
    for name in backend mcp frontend; do
      case "$(image_state "$name" "$version")" in
        ready) ;;
        missing) not_ready+="${not_ready:+, }$name" ;;
        denied) denied+="${denied:+, }$name" ;;
        *) unknown+="${unknown:+, }$name" ;;
      esac
    done
    if [ -n "$unknown" ]; then
      warn "could not check ghcr.io for $unknown (network or registry trouble); will try again"
      exit 0
    fi
    if [ -n "$denied" ]; then
      log "images not ready: ghcr.io/$ghcr_owner/socialos-{$denied}:$version are not visible without a login (not published yet, or private packages: set AUTOUPDATE_CHECK_IMAGES=false to skip this check); will try again"
      exit 0
    fi
    if [ -n "$not_ready" ]; then
      log "images not ready: ghcr.io/$ghcr_owner/socialos-{$not_ready}:$version do not exist yet (the Release workflow may still be building them); will try again"
      exit 0
    fi
    ;;
esac

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
