#!/usr/bin/env bash
# Put the staged Steerpost sites into the host's Caddy, safely. Run as root, after ./host-proxy/render-caddy.sh has staged
# /opt/socialos/caddy/staging/socialos.caddy. The host Caddyfile gets one line: `import /opt/socialos/caddy/*.caddy`.
#
#   sudo ./host-proxy/install-caddy-import.sh                 # URLs to check come from HOST_PROXY_CHECK_URLS
#   sudo ./host-proxy/install-caddy-import.sh --no-checks     # no URL checks (not recommended on a shared host)
#
# HOST_PROXY_CHECK_URLS (environment, or in /opt/socialos/.env): space-separated URLs of the sites the host already
# serves, e.g. "https://www.example.org/". Each must answer with a status below 500 before and after the change.
#
# Order (safe to repeat): prerequisites and URL checks (nothing is changed up to here) -> timestamped backup of the
# Caddyfile -> copy the staged snippets into /opt/socialos/caddy (the folder the import line matches) -> append the import
# line if it is missing -> `caddy validate` -> `systemctl reload caddy` -> check the URLs again. If anything fails, or the
# script is interrupted, it puts back the Caddyfile from the backup and the snippet set Caddy last loaded successfully
# (files that are not in that set are deleted), reloads Caddy, checks the URLs again and says what state the host is in.
# Only /etc/caddy/Caddyfile (plus its backup) and /opt/socialos/caddy are ever written.
# Tunables: CADDYFILE (/etc/caddy/Caddyfile), CADDY_BIN (caddy), CHECK_TRIES (5), CHECK_DELAY (2 seconds).
# Exit codes: 0 done; 1 failed and rolled back, host verified; 2 cannot start, nothing changed; 3 a check URL was already
# failing, nothing changed; 4 failed and the rollback could not be completed or verified: read the last messages.
set -Eeuo pipefail
cd /
shopt -s nullglob

SOCIALOS_DIR=${SOCIALOS_DIR:-/opt/socialos}
CADDYFILE=${CADDYFILE:-/etc/caddy/Caddyfile}
CADDY_BIN=${CADDY_BIN:-caddy}
CHECK_TRIES=${CHECK_TRIES:-5}
CHECK_DELAY=${CHECK_DELAY:-2}
SNIPPET_DIR=$SOCIALOS_DIR/caddy      # matched by the import line: only this script writes here
STAGING_DIR=$SNIPPET_DIR/staging     # render-caddy.sh writes here; a subfolder, so the import line does not match it
APPLIED_DIR=$SNIPPET_DIR/.applied    # the snippet set that Caddy last loaded successfully
IMPORT_LINE="import $SNIPPET_DIR/*.caddy"

log() { printf 'install-caddy-import: %s\n' "$*"; }
warn() { printf 'install-caddy-import: %s\n' "$*" >&2; }
die() {
  warn "$1"
  exit "${2:-2}"
}
usage() { sed -n '2,19p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; }

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
if [ "$(id -u)" -ne 0 ] && [ "${ALLOW_NON_ROOT:-0}" != 1 ]; then die "run as root (sudo). Nothing was changed."; fi
[ -f "$CADDYFILE" ] || die "$CADDYFILE not found: is Caddy installed on this host? Nothing was changed."
for cmd in "$CADDY_BIN" systemctl curl; do
  command -v "$cmd" >/dev/null 2>&1 || die "$cmd not found in PATH. Nothing was changed."
done

# Whatever is in these folders becomes configuration of the host's Caddy, which also serves other sites. If anyone but
# the user running this script can write there, that someone could change the host's Caddy configuration.
require_private() {
  local path=$1 owner mode
  owner=$(stat -c %u "$path") || die "cannot stat $path. Nothing was changed."
  mode=$(stat -c %a "$path") || die "cannot stat $path. Nothing was changed."
  [ "$owner" = "$(id -u)" ] || die "$path belongs to uid $owner instead of the user running this script. Keep $SNIPPET_DIR root-owned (do not chown it to a deploy user). Nothing was changed."
  if (((8#$mode & 8#022) != 0)); then
    die "$path is writable by group or others (mode $mode). Use mode 755 and root ownership: chmod go-w $path. Nothing was changed."
  fi
}
for path in "$SOCIALOS_DIR" "$SNIPPET_DIR" "$STAGING_DIR" "$APPLIED_DIR" "$SNIPPET_DIR"/*.caddy "$STAGING_DIR"/*.caddy "$APPLIED_DIR"/*.caddy; do
  if [ -e "$path" ]; then require_private "$path"; fi
done
staged=("$STAGING_DIR"/*.caddy)
[ "${#staged[@]}" -gt 0 ] || die "nothing is staged in $STAGING_DIR: run ./host-proxy/render-caddy.sh first. Nothing was changed."

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
  die "HOST_PROXY_CHECK_URLS is empty. Name at least one site this host already serves (it is checked before and after the change), or pass --no-checks. Nothing was changed."
fi
if [ "$no_checks" = true ]; then urls=""; fi
read -r -a check_urls <<<"$urls"
for url in "${check_urls[@]}"; do
  [[ "$url" =~ ^https?://[^[:space:]]+$ ]] || die "not an http(s) URL in HOST_PROXY_CHECK_URLS: $url. Nothing was changed."
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

# append_import: the import line goes at the end of the Caddyfile, on its own line.
append_import() {
  if [ -n "$(tail -c 1 "$CADDYFILE")" ]; then printf '\n' >>"$CADDYFILE" || return 1; fi
  printf '\n# Steerpost (added by %s)\n%s\n' "$SOCIALOS_DIR/host-proxy/install-caddy-import.sh" "$IMPORT_LINE" >>"$CADDYFILE"
}

# set_names DIR: the names of the *.caddy files in DIR, space separated.
set_names() {
  local f out=""
  for f in "$1"/*.caddy; do out+="${out:+ }${f##*/}"; done
  printf '%s' "$out"
}

# sync_set SRC DST: DST/*.caddy becomes an exact copy of SRC/*.caddy (files that are not in SRC are deleted). Each file is
# copied under a hidden name first and renamed, so a half-written file never matches the import glob. A missing SRC is an
# empty set. Returns 1 if anything could not be done.
sync_set() {
  local f name tmp rc=0
  mkdir -p "$2" || return 1
  for f in "$1"/*.caddy; do
    name=${f##*/}
    tmp=$2/.$name.new.$$
    if cp -p "$f" "$tmp" && mv -f "$tmp" "$2/$name"; then :; else
      rm -f "$tmp"
      rc=1
    fi
  done
  for f in "$2"/*.caddy; do
    if [ ! -e "$1/${f##*/}" ]; then rm -f "$f" || rc=1; fi
  done
  return "$rc"
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

# ---- rollback ---------------------------------------------------------------------------------------------------
backup=""
reloaded_new=false # has Caddy been told to load the new configuration?
rolling_back=false
fail_reason=""

# rollback REASON: puts back the Caddyfile and the last applied snippet set, reloads, verifies, tells the truth, exits.
rollback() {
  local files_ok=true reload_ok=false checks_ok=true rc=1 set_after
  [ "$rolling_back" = false ] || return 0
  rolling_back=true
  set +e
  trap '' INT TERM HUP
  warn "FAILED: $1"
  cp -p "$backup" "$CADDYFILE" || {
    files_ok=false
    warn "COULD NOT RESTORE $CADDYFILE from $backup. Copy it back by hand: cp -p $backup $CADDYFILE"
  }
  sync_set "$APPLIED_DIR" "$SNIPPET_DIR" || {
    files_ok=false
    warn "COULD NOT RESTORE the snippet set in $SNIPPET_DIR from $APPLIED_DIR."
  }
  if [ "$files_ok" = true ]; then
    set_after=$(set_names "$SNIPPET_DIR")
    warn "restored $CADDYFILE from $backup and the snippet set in $SNIPPET_DIR to the last applied one (${set_after:-none})"
  fi
  if systemctl reload caddy; then reload_ok=true; fi
  if [ "$reload_ok" = true ]; then
    warn "Caddy reloaded: it runs the restored configuration."
  elif [ "$reloaded_new" = true ]; then
    warn "CADDY IS STILL RUNNING THE NEW CONFIGURATION: the reload after the restore failed. Run 'systemctl reload caddy' now, then read: journalctl -u caddy -n 50"
  else
    warn "Caddy could not be reloaded after the restore. It never loaded the new configuration, so it still runs the one it had before this run."
  fi
  if [ "${#check_urls[@]}" -gt 0 ]; then
    if check_with_retries; then
      warn "the ${#check_urls[@]} check URL(s) answer."
    else
      checks_ok=false
      warn "the check URL(s) above STILL FAIL after the restore."
    fi
  fi
  if [ "$files_ok" = true ] && { [ "$reload_ok" = true ] || [ "$reloaded_new" = false ]; } && [ "$checks_ok" = true ]; then
    warn "ROLLED BACK. The host is as it was before this run; Steerpost is not routed (or still on its last applied version)."
  else
    rc=4
    warn "ROLLBACK NOT VERIFIED: act on the messages above before anything else."
  fi
  exit "$rc"
}

# ---- apply ------------------------------------------------------------------------------------------------------
if [ "${#check_urls[@]}" -gt 0 ]; then
  log "checking ${#check_urls[@]} URL(s) before changing anything"
  check_once || die "a check URL is not healthy before the change. Nothing was changed. Fix that first (or pass --no-checks)." 3
else
  warn "no URL checks: the other sites on this host are not verified"
fi

backup="$CADDYFILE.socialos-backup-$(date -u +%Y%m%dT%H%M%SZ)"
cp -p "$CADDYFILE" "$backup" || die "could not back up $CADDYFILE to $backup. Nothing was changed."
log "backup: $backup"
trap 'rollback "interrupted by a signal"' INT TERM HUP

# apply: every step ends in `|| return 1`; the reason goes to $fail_reason.
apply() {
  if ! sync_set "$STAGING_DIR" "$SNIPPET_DIR"; then
    fail_reason="could not copy the staged snippets into $SNIPPET_DIR"
    return 1
  fi
  log "swapped in the staged snippets: $(set_names "$SNIPPET_DIR")"
  if import_present; then
    log "the import line is already in $CADDYFILE"
  else
    if ! append_import; then
      fail_reason="could not append the import line to $CADDYFILE"
      return 1
    fi
    log "appended: $IMPORT_LINE"
  fi
  if ! validate_config; then
    fail_reason="the new configuration is not valid (it was not loaded)"
    return 1
  fi
  log "caddy validate: ok"
  reloaded_new=true # from here on Caddy may be running the new configuration, even if the reload reports an error
  if ! systemctl reload caddy; then
    reloaded_new=false # a reload that fails leaves the previous configuration running
    fail_reason="systemctl reload caddy failed (Caddy kept its previous configuration)"
    return 1
  fi
  log "caddy reloaded"
  if [ "${#check_urls[@]}" -gt 0 ]; then
    if ! check_with_retries; then
      fail_reason="a site that was healthy before no longer answers after the reload"
      return 1
    fi
    log "all check URLs still answer"
  fi
}
apply || rollback "$fail_reason"

# ---- commit -----------------------------------------------------------------------------------------------------
trap - INT TERM HUP
sync_set "$SNIPPET_DIR" "$APPLIED_DIR" || die "applied, but could not record the applied snippet set in $APPLIED_DIR (a later rollback would restore an older set). Caddy is running the new configuration." 1
log "done: Caddy serves the staged snippets ($(set_names "$SNIPPET_DIR")). To undo: cp -p $backup $CADDYFILE && systemctl reload caddy"
