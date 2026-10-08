#!/usr/bin/env bash
# Add the one line `import /opt/socialos/caddy/*.caddy` to the host's Caddyfile, safely. Run as root, after
# ./host-proxy/render-caddy.sh has written /opt/socialos/caddy/socialos.caddy.
#
#   sudo ./host-proxy/install-caddy-import.sh                 # URLs to check come from HOST_PROXY_CHECK_URLS
#   sudo ./host-proxy/install-caddy-import.sh --no-checks     # no URL checks (not recommended on a shared host)
#
# HOST_PROXY_CHECK_URLS (environment, or in /opt/socialos/.env): space-separated URLs of the sites the host already
# serves, e.g. "https://www.example.org/". Each must answer with a status below 500 before and after the change.
#
# Steps (safe to repeat): check the URLs -> back up the Caddyfile (timestamped copy next to it) -> append the import
# line if it is missing -> `caddy validate` -> `systemctl reload caddy` -> check the URLs again. If validation, the
# reload or a check fails, the backup is restored (and the previous SocialOS snippet), caddy is reloaded again and the
# script exits non-zero. Nothing else on the host is touched.
# Tunables: CADDYFILE (/etc/caddy/Caddyfile), CADDY_BIN (caddy), CHECK_TRIES (5), CHECK_DELAY (2 seconds).
# Exit codes: 0 done, 1 rolled back, 2 cannot start (usage, missing prerequisite), 3 a check URL was already failing.
set -Eeuo pipefail
cd /

SOCIALOS_DIR=${SOCIALOS_DIR:-/opt/socialos}
CADDYFILE=${CADDYFILE:-/etc/caddy/Caddyfile}
CADDY_BIN=${CADDY_BIN:-caddy}
CHECK_TRIES=${CHECK_TRIES:-5}
CHECK_DELAY=${CHECK_DELAY:-2}
SNIPPET_DIR=$SOCIALOS_DIR/caddy
APPLIED_DIR=$SNIPPET_DIR/.applied # last snippets that Caddy loaded successfully (not matched by the *.caddy glob)
IMPORT_LINE="import $SNIPPET_DIR/*.caddy"

log() { printf 'install-caddy-import: %s\n' "$*"; }
warn() { printf 'install-caddy-import: %s\n' "$*" >&2; }
die() {
  warn "$*"
  exit "${2:-2}"
}
usage() { sed -n '2,16p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; }

no_checks=false
while [ $# -gt 0 ]; do
  case "$1" in
    --no-checks) no_checks=true ;;
    -h | --help)
      usage
      exit 0
      ;;
    *) die "unknown argument: $1" ;;
  esac
  shift
done

# ---- prerequisites (nothing is changed before all of them hold) --------------------------------------------------
if [ "$(id -u)" -ne 0 ] && [ "${ALLOW_NON_ROOT:-0}" != 1 ]; then die "run as root (sudo)"; fi
[ -f "$CADDYFILE" ] || die "$CADDYFILE not found: is Caddy installed on this host?"
for cmd in "$CADDY_BIN" systemctl curl; do
  command -v "$cmd" >/dev/null 2>&1 || die "$cmd not found in PATH"
done
compgen -G "$SNIPPET_DIR/*.caddy" >/dev/null || die "no $SNIPPET_DIR/*.caddy yet: run ./host-proxy/render-caddy.sh first"

# env_get KEY: last assignment of KEY in /opt/socialos/.env, without surrounding quotes.
env_get() {
  local line
  [ -f "$SOCIALOS_DIR/.env" ] || return 0
  line=$(grep -E "^[[:space:]]*$1=" "$SOCIALOS_DIR/.env" | tail -n 1 || true)
  line=${line#*=}
  line=${line%\"}
  line=${line#\"}
  line=${line%\'}
  line=${line#\'}
  printf '%s' "$line"
}

urls=${HOST_PROXY_CHECK_URLS-}
[ -n "$urls" ] || urls=$(env_get HOST_PROXY_CHECK_URLS)
if [ -z "$urls" ] && [ "$no_checks" = false ]; then
  die "HOST_PROXY_CHECK_URLS is empty. Name at least one site this host already serves (it is checked before and after the change), or pass --no-checks"
fi
if [ "$no_checks" = true ]; then urls=""; fi
read -r -a check_urls <<<"$urls"
for url in "${check_urls[@]}"; do
  [[ "$url" =~ ^https?://[^[:space:]]+$ ]] || die "not an http(s) URL in HOST_PROXY_CHECK_URLS: $url"
done

# ---- helpers ---------------------------------------------------------------------------------------------------
status_of() { curl -sS -o /dev/null -w '%{http_code}' --max-time 15 "$1" 2>/dev/null || true; }
healthy() { [[ "$1" =~ ^[1-4][0-9][0-9]$ ]]; }

# check_once: warns about every URL that does not answer with a status below 500; returns 1 if there is any.
check_once() {
  local url status bad=0
  for url in "${check_urls[@]}"; do
    status=$(status_of "$url")
    if ! healthy "$status"; then
      warn "check failed: $url answered ${status:-000}"
      bad=1
    fi
  done
  return "$bad"
}

# check_with_retries: after a reload a site can need a moment, so try a few times before giving up.
check_with_retries() {
  local try
  for ((try = 1; try <= CHECK_TRIES; try++)); do
    if check_once 2>/dev/null; then return 0; fi
    if [ "$try" -lt "$CHECK_TRIES" ]; then sleep "$CHECK_DELAY"; fi
  done
  check_once # prints which URL is still failing
}

import_present() {
  local normalized
  normalized=$(sed 's/^[[:space:]]*//; s/[[:space:]]*$//' "$CADDYFILE")
  grep -Fxq "$IMPORT_LINE" <<<"$normalized"
}

validate_config() {
  local out rc=0
  # As the user that runs the service when possible, so that a file Caddy cannot read is found now, not at reload.
  if id -u caddy >/dev/null 2>&1 && command -v runuser >/dev/null 2>&1 && [ "$(id -u)" -eq 0 ]; then
    out=$(runuser -u caddy -- "$CADDY_BIN" validate --config "$CADDYFILE" --adapter caddyfile 2>&1) || rc=$?
  else
    out=$("$CADDY_BIN" validate --config "$CADDYFILE" --adapter caddyfile 2>&1) || rc=$?
  fi
  if [ "$rc" -ne 0 ]; then
    warn "caddy validate failed:"
    printf '%s\n' "$out" | tail -n 15 >&2
  fi
  return "$rc"
}

backup=""
rollback() {
  local why=$1 snapshot
  set +e # best effort from here on: every restore step runs even if an earlier one fails
  warn "$why: restoring $backup"
  cp -p "$backup" "$CADDYFILE" || warn "COULD NOT RESTORE $CADDYFILE from $backup: copy it back by hand"
  if [ -d "$APPLIED_DIR" ]; then
    for snapshot in "$APPLIED_DIR"/*.caddy; do
      if [ -f "$snapshot" ]; then cp -p "$snapshot" "$SNIPPET_DIR/$(basename "$snapshot")"; fi
    done
  fi
  if systemctl reload caddy; then
    warn "Caddy reloaded with the previous configuration. The host is as it was; SocialOS is not routed."
  else
    warn "Caddy could not be reloaded after the restore. Check now: systemctl status caddy; journalctl -u caddy -n 50"
  fi
  exit 1
}

# ---- apply ------------------------------------------------------------------------------------------------------
if [ "${#check_urls[@]}" -gt 0 ]; then
  log "checking ${#check_urls[@]} URL(s) before changing anything"
  check_once || die "a check URL is not healthy before the change, so nothing was touched. Fix that first (or pass --no-checks)" 3
else
  warn "no URL checks: the other sites on this host are not verified"
fi

backup="$CADDYFILE.socialos-backup-$(date -u +%Y%m%dT%H%M%SZ)"
cp -p "$CADDYFILE" "$backup"
log "backup: $backup"

if import_present; then
  log "the import line is already in $CADDYFILE"
else
  [ -z "$(tail -c 1 "$CADDYFILE")" ] || printf '\n' >>"$CADDYFILE"
  printf '\n# SocialOS (added by %s)\n%s\n' "$SOCIALOS_DIR/host-proxy/install-caddy-import.sh" "$IMPORT_LINE" >>"$CADDYFILE"
  log "appended: $IMPORT_LINE"
fi

validate_config || rollback "the configuration is not valid"
log "caddy validate: ok"

systemctl reload caddy || rollback "systemctl reload caddy failed"
log "caddy reloaded"

if [ "${#check_urls[@]}" -gt 0 ]; then
  check_with_retries || rollback "a site that was healthy before no longer answers"
  log "all check URLs still answer"
fi

mkdir -p "$APPLIED_DIR"
for snippet in "$SNIPPET_DIR"/*.caddy; do cp -p "$snippet" "$APPLIED_DIR/"; done
log "done. To undo: cp -p $backup $CADDYFILE && systemctl reload caddy"
