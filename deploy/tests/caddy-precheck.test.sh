#!/usr/bin/env bash
# host-proxy/caddy-precheck.sh against a stubbed caddy: a broken SocialOS snippet is moved aside so that Caddy can start,
# a problem that is not ours is left alone, and the script always exits 0.
# shellcheck source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
shopt -s nullglob

# setup: a Caddyfile that imports $LIVE/*.caddy and one valid snippet. The caddy stub fails when the Caddyfile or a live
# snippet contains INVALID, and hangs when $SB/hang exists.
setup() {
  new_sb
  LIVE=$SB/opt/caddy
  mkdir -p "$SB/etc" "$LIVE"
  printf 'example.org {\n\trespond "other site"\n}\nimport %s/*.caddy\n' "$LIVE" >"$SB/etc/Caddyfile"
  printf 'app.example.com {\n\trespond "ok"\n}\n' >"$LIVE/socialos.caddy"
  cat >"$SB/bin/caddy" <<STUB
#!/usr/bin/env bash
echo "caddy \$*" >>"$SB/calls"
[ ! -e "$SB/hang" ] || sleep 5
if grep -q INVALID "$SB/etc/Caddyfile" "$LIVE"/*.caddy 2>/dev/null; then echo "Error: adapting config: INVALID" >&2; exit 1; fi
STUB
  chmod +x "$SB/bin/caddy"
}
precheck() {
  rc=0
  out=$(PATH="$SB/bin:$PATH" SOCIALOS_DIR="$SB/opt" CADDYFILE="$SB/etc/Caddyfile" VALIDATE_TIMEOUT=2 \
    bash "$REPO_DEPLOY/host-proxy/caddy-precheck.sh" 2>&1) || rc=$?
}
live() { local f s=""; for f in "$LIVE"/*.caddy; do s+="${f##*/} "; done; printf '%s' "${s% }"; }
quarantined() { local f n=0; for f in "$LIVE"/quarantine/*/*.caddy; do n=$((n + 1)); done; echo "$n"; }

# 1. valid: nothing moves
setup
precheck
assert_eq "valid: exit" 0 "$rc"
assert_eq "valid: snippet stays" socialos.caddy "$(live)"
assert_has "valid: says so" "$out" "is valid"

# 2. our snippet is broken: it is moved aside, Caddy validates without it, an alert is left
setup
echo INVALID >>"$LIVE/socialos.caddy"
precheck
assert_eq "broken snippet: exit 0" 0 "$rc"
assert_eq "broken snippet: nothing imported any more" "" "$(live)"
assert_eq "broken snippet: quarantined" 1 "$(quarantined)"
assert_has "broken snippet: explained" "$out" "Caddy starts without the SocialOS sites"
assert_file "broken snippet: alert" "$SB/opt/.deploy/guard/alerts/caddy-quarantine"
assert_eq "broken snippet: validated twice" 2 "$(grep -c '^caddy validate' "$SB/calls")"

# 3. the host Caddyfile itself is broken: not ours, our snippet goes back, nothing else changes
setup
echo INVALID >>"$SB/etc/Caddyfile"
precheck
assert_eq "host Caddyfile broken: exit 0" 0 "$rc"
assert_eq "host Caddyfile broken: snippet back in place" socialos.caddy "$(live)"
assert_eq "host Caddyfile broken: nothing quarantined" 0 "$(quarantined)"
assert_has "host Caddyfile broken: explained" "$out" "not ours to fix"
assert_no_file "host Caddyfile broken: no alert" "$SB/opt/.deploy/guard/alerts/caddy-quarantine"

# 4. nothing imported (folder empty or deleted): nothing to validate
setup
rm "$LIVE/socialos.caddy"
precheck
assert_eq "no snippet: exit" 0 "$rc"
assert_no_file "no snippet: caddy not called" "$SB/calls"
rm -rf "$LIVE"
precheck
assert_eq "folder deleted: exit" 0 "$rc"

# 5. caddy validate hangs: time limit, nothing moved, exit 0
setup
touch "$SB/hang"
precheck
assert_eq "hang: exit 0" 0 "$rc"
assert_eq "hang: snippet stays" socialos.caddy "$(live)"
assert_has "hang: explained" "$out" "timed out"

finish
