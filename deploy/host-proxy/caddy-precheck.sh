#!/usr/bin/env bash
# Runs right before the host's Caddy starts (systemd/socialos-caddy-precheck.service: Before=caddy.service, pulled in by a
# weak Wants=). Its only job: a broken SocialOS snippet must never keep Caddy, and with it the host's other sites, from
# starting, after a reboot, a `systemctl restart caddy` or a Caddy package upgrade.
#
#   1. no SocialOS snippet in /opt/socialos/caddy: nothing to do (an import glob that matches nothing is fine for Caddy);
#   2. `caddy validate` of the host Caddyfile passes: nothing to do;
#   3. it fails: the SocialOS snippets move to /opt/socialos/caddy/quarantine/<UTC time>/ (not matched by the import glob)
#      and Caddy is validated again. Valid now: they stay there, an alert is left in .deploy/guard/alerts/caddy-quarantine,
#      and Caddy starts without the SocialOS sites. Still invalid: the problem is not ours, so the snippets go back and
#      nothing else is changed.
# A run interrupted while the snippets were aside (marker .in-progress in their quarantine folder) is undone by the next
# run before anything else; a quarantine that has no alert gets one.
# It always exits 0 and is time-limited, so it can never block or delay Caddy for long. To bring the SocialOS sites back:
# fix the cause, then host-proxy/render-caddy.sh and host-proxy/install-caddy-import.sh (which validates before it reloads).
# Tunables: SOCIALOS_DIR (/opt/socialos), CADDYFILE (/etc/caddy/Caddyfile), CADDY_BIN (caddy), VALIDATE_TIMEOUT (15 s).
set -Euo pipefail # no -e: every failure is handled, and the script must reach its exit 0
cd /
shopt -s nullglob

SOCIALOS_DIR=${SOCIALOS_DIR:-/opt/socialos}
CADDYFILE=${CADDYFILE:-/etc/caddy/Caddyfile}
CADDY_BIN=${CADDY_BIN:-caddy}
VALIDATE_TIMEOUT=${VALIDATE_TIMEOUT:-15}
SNIPPET_DIR=$SOCIALOS_DIR/caddy
ALERT_DIR=$SOCIALOS_DIR/.deploy/guard/alerts

P_ERR=""
if [ -n "${JOURNAL_STREAM-}" ]; then P_ERR="<3>"; fi
log() { printf 'caddy-precheck: %s\n' "$*"; }
error() { printf '%scaddy-precheck: %s\n' "$P_ERR" "$*"; }

# validate: the same check install-caddy-import.sh runs, as the caddy user when it exists (it reads the files at start).
validate() {
  local cmd=(timeout "$VALIDATE_TIMEOUT" "$CADDY_BIN" validate --config "$CADDYFILE" --adapter caddyfile)
  if id -u caddy >/dev/null 2>&1 && command -v runuser >/dev/null 2>&1 && [ "$(id -u)" -eq 0 ]; then
    cmd=(runuser -u caddy -- "${cmd[@]}")
  fi
  "${cmd[@]}" >"$1" 2>&1
}

# quarantine_alert MESSAGE: the marker the guard's --status and the README point to.
quarantine_alert() {
  mkdir -p "$ALERT_DIR" && printf 'since %s: %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$1" >"$ALERT_DIR/caddy-quarantine"
}

# recover: puts back the snippets of an interrupted run (a live file of the same name, newer, wins) and makes sure every
# finished quarantine is alerted.
recover() {
  local dir f left
  for dir in "$SNIPPET_DIR"/quarantine/*/; do
    dir=${dir%/}
    if [ -e "$dir/.in-progress" ]; then
      for f in "$dir"/*.caddy; do
        if [ ! -e "$SNIPPET_DIR/${f##*/}" ]; then mv -f "$f" "$SNIPPET_DIR/"; fi
      done
      rm -f "$dir/.in-progress"
      left=("$dir"/*.caddy)
      if [ "${#left[@]}" -eq 0 ] && rmdir "$dir" 2>/dev/null; then
        log "put back the snippets of an interrupted run from $dir"
      else
        error "an interrupted run left snippets in $dir that could not all be put back"
        quarantine_alert "snippets of an interrupted run left in $dir"
      fi
    elif [ -n "$(find "$dir" -maxdepth 1 -name '*.caddy' -print -quit)" ] && [ ! -e "$ALERT_DIR/caddy-quarantine" ]; then
      quarantine_alert "SocialOS Caddy snippet quarantined in $dir"
    fi
  done
}

recover
snippets=("$SNIPPET_DIR"/*.caddy)
if [ "${#snippets[@]}" -eq 0 ]; then
  log "no SocialOS snippet is imported: nothing to check"
  exit 0
fi
out=$(mktemp) || exit 0
trap 'rm -f "$out"' EXIT

validate "$out"
rc=$?
if [ "$rc" -eq 0 ]; then
  log "the Caddyfile with the SocialOS snippet(s) is valid"
  exit 0
elif [ "$rc" -eq 124 ]; then
  error "caddy validate timed out after ${VALIDATE_TIMEOUT}s: leaving everything as it is"
  exit 0
fi

quarantine=$SNIPPET_DIR/quarantine/$(date -u +%Y%m%dT%H%M%SZ)
if ! mkdir -p "$quarantine" || ! chmod 700 "$quarantine" || ! touch "$quarantine/.in-progress" ||
  ! mv -f "${snippets[@]}" "$quarantine/"; then
  error "the Caddyfile is invalid and the SocialOS snippets could not be moved aside: Caddy may fail to start"
  tail -n 5 "$out"
  exit 0
fi

if validate "$out"; then
  error "the SocialOS snippet(s) made the Caddyfile invalid: moved to $quarantine, Caddy starts without the SocialOS sites"
  quarantine_alert "SocialOS Caddy snippet quarantined in $quarantine"
  rm -f "$quarantine/.in-progress"
  exit 0
fi

if ! mv -f "$quarantine"/*.caddy "$SNIPPET_DIR/" || ! rm -f "$quarantine/.in-progress" || ! rmdir "$quarantine"; then
  error "could not put the SocialOS snippets back from $quarantine"
  quarantine_alert "snippets could not be put back from $quarantine"
fi
error "the Caddyfile is invalid even without the SocialOS snippets: not ours to fix, left as it was. caddy validate said:"
tail -n 5 "$out"
exit 0
