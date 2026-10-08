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
if ! mkdir -p "$quarantine" || ! chmod 700 "$quarantine" || ! mv -f "${snippets[@]}" "$quarantine/"; then
  error "the Caddyfile is invalid and the SocialOS snippets could not be moved aside: Caddy may fail to start"
  tail -n 5 "$out"
  exit 0
fi

if validate "$out"; then
  error "the SocialOS snippet(s) made the Caddyfile invalid: moved to $quarantine, Caddy starts without the SocialOS sites"
  mkdir -p "$ALERT_DIR" &&
    printf 'since %s: SocialOS Caddy snippet quarantined in %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$quarantine" \
      >"$ALERT_DIR/caddy-quarantine"
  exit 0
fi

mv -f "$quarantine"/*.caddy "$SNIPPET_DIR/" && rmdir "$quarantine"
error "the Caddyfile is invalid even without the SocialOS snippets: not ours to fix, left as it was. caddy validate said:"
tail -n 5 "$out"
exit 0
