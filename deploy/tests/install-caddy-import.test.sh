#!/usr/bin/env bash
# host-proxy/install-caddy-import.sh against stubbed caddy, systemctl and curl: staged snippets go live only through the
# installer, and every failure puts the previous state back and says what state the host is in.
# shellcheck source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
shopt -s nullglob # a glob with no match expands to nothing (live_files, backups)

# setup: sandbox "host" with a Caddyfile, a staged snippet and stubs.
#   caddy validate  fails when the Caddyfile or a live snippet contains the word INVALID; sleeps when $SB/slow exists
#   systemctl       records the call; reload fails when $SB/reload.fail exists, or from the Nth reload on ($SB/reload.fail-from);
#                   a reload flips the check URL to 503 and the next one back to 200 ($SB/flip), or keeps it at 503 ($SB/flip-always)
#   curl            prints the status in $SB/status (200 unless a test changes it)
setup() {
  new_sb
  LIVE=$SB/opt-root/caddy
  STAGED=$LIVE/staging
  mkdir -p "$SB/etc" "$STAGED"
  chmod 755 "$SB/opt-root" "$LIVE" "$STAGED"
  printf '{\n\temail ops@example.com\n}\n\nexample.org {\n\trespond "other site"\n}\n' >"$SB/etc/Caddyfile"
  printf 'app.example.com {\n\trespond "ok"\n}\n' >"$STAGED/socialos.caddy"
  chmod 644 "$STAGED/socialos.caddy"
  echo 200 >"$SB/status"
  cat >"$SB/bin/caddy" <<STUB
#!/usr/bin/env bash
echo "caddy \$*" >>"$SB/calls"
[ -e "$SB/slow" ] && sleep 3
if grep -q INVALID "$SB/etc/Caddyfile" "$LIVE"/*.caddy 2>/dev/null; then echo "Error: adapting config: INVALID" >&2; exit 1; fi
STUB
  cat >"$SB/bin/systemctl" <<STUB
#!/usr/bin/env bash
echo "systemctl \$*" >>"$SB/calls"
n=\$(grep -c '^systemctl reload' "$SB/calls")
[ -e "$SB/reload.fail" ] && exit 1
if [ -e "$SB/reload.fail-from" ] && [ "\$n" -ge "\$(cat "$SB/reload.fail-from")" ]; then exit 1; fi
if [ -e "$SB/flip-always" ]; then echo 503 >"$SB/status"
elif [ -e "$SB/flip" ]; then
  if [ "\$(cat "$SB/status")" = 503 ]; then echo 200 >"$SB/status"; else echo 503 >"$SB/status"; fi
fi
exit 0
STUB
  cat >"$SB/bin/curl" <<STUB
#!/usr/bin/env bash
cat "$SB/status"
STUB
  chmod +x "$SB"/bin/*
  orig=$(cat "$SB/etc/Caddyfile")
}
# install [ARGS]: runs the script as the "host"; sets $out and $rc.
install() {
  rc=0
  out=$(PATH="$SB/bin:$PATH" ALLOW_NON_ROOT=1 SOCIALOS_DIR="$SB/opt-root" CADDYFILE="$SB/etc/Caddyfile" CHECK_TRIES=2 CHECK_DELAY=0 \
    bash "$REPO_DEPLOY/host-proxy/install-caddy-import.sh" "$@" 2>&1) || rc=$?
}
live_files() { local f out=""; for f in "$LIVE"/*.caddy; do out+="${f##*/} "; done; printf '%s' "${out% }"; }
imports() { grep -c "^import $LIVE/\*.caddy\$" "$SB/etc/Caddyfile" || true; }
reloads() { grep -c '^systemctl reload caddy' "$SB/calls" || true; }
backups() {
  local n=0 f
  for f in "$SB"/etc/Caddyfile.socialos-backup-*; do
    if [ -e "$f" ]; then n=$((n + 1)); fi
  done
  echo "$n"
}
URLS="https://www.example.org/ https://example.net/health"

# 1. first install: the staged snippet goes live only now; backup, import line, validate, reload, checks, snapshot
setup
assert_eq "staged only: nothing live before the install" "" "$(live_files)"
HOST_PROXY_CHECK_URLS="$URLS" install
assert_eq "first install: exit" 0 "$rc"
assert_eq "first install: snippet is live" "socialos.caddy" "$(live_files)"
assert_eq "first install: live equals staged" "$(cat "$STAGED/socialos.caddy")" "$(cat "$LIVE/socialos.caddy")"
assert_file "first install: the staged copy is kept for the next run" "$STAGED/socialos.caddy"
assert_eq "import line appended once" 1 "$(imports)"
assert_eq "one timestamped backup" 1 "$(backups)"
assert_eq "backup equals the original" "$orig" "$(cat "$SB"/etc/Caddyfile.socialos-backup-*)"
assert_has "validated the right way" "$(cat "$SB/calls")" "caddy validate --config $SB/etc/Caddyfile --adapter caddyfile"
assert_eq "reloaded once" 1 "$(reloads)"
assert_eq "last applied set recorded" "$(cat "$LIVE/socialos.caddy")" "$(cat "$LIVE/.applied/socialos.caddy")"
assert_eq "the original content is untouched above the import" "$orig" "$(head -n "$(wc -l <<<"$orig")" "$SB/etc/Caddyfile")"

# 2. idempotent: second run adds no second import line
HOST_PROXY_CHECK_URLS="$URLS" install
assert_eq "second run: exit" 0 "$rc"
assert_eq "second run: still one import line" 1 "$(imports)"
assert_has "second run: says so" "$out" "already in"

# 3. a file without a trailing newline; an indented import line that is already there
setup
printf 'example.org {\n\trespond "x"\n}' >"$SB/etc/Caddyfile"
HOST_PROXY_CHECK_URLS="$URLS" install
assert_eq "no trailing newline: exit" 0 "$rc"
assert_eq "no trailing newline: import on its own line" 1 "$(imports)"
setup
printf 'example.org {\n}\n\t%s  \n' "import $LIVE/*.caddy" >"$SB/etc/Caddyfile"
HOST_PROXY_CHECK_URLS="$URLS" install
assert_eq "existing indented import: exit" 0 "$rc"
assert_eq "existing indented import: not added again" 0 "$(grep -c '^import' "$SB/etc/Caddyfile")"

# 4. the staged snippet is invalid on a first install: nothing stays imported, the Caddyfile is back, the verdict is clear
setup
echo "INVALID" >>"$STAGED/socialos.caddy"
HOST_PROXY_CHECK_URLS="$URLS" install
assert_eq "invalid, first install: exit" 1 "$rc"
assert_eq "invalid, first install: Caddyfile restored" "$orig" "$(cat "$SB/etc/Caddyfile")"
assert_eq "invalid, first install: no snippet left in the imported folder" "" "$(live_files)"
assert_has "invalid, first install: validator output shown" "$out" "INVALID"
assert_has "invalid, first install: reason" "$out" "FAILED: the new configuration is not valid (it was not loaded)"
assert_has "invalid, first install: restore reported" "$out" "restored $SB/etc/Caddyfile from"
assert_has "invalid, first install: verdict" "$out" "ROLLED BACK"
assert_eq "invalid, first install: one reload, after the restore" 1 "$(reloads)"
assert_no_file "invalid, first install: nothing recorded as applied" "$LIVE/.applied/socialos.caddy"

# 5. broken staged snippet on a re-run: the live set goes back to exactly the last applied one (strays are deleted)
setup
HOST_PROXY_CHECK_URLS="$URLS" install
good_snippet=$(cat "$LIVE/socialos.caddy")
good_caddyfile=$(cat "$SB/etc/Caddyfile")
echo "stray" >"$LIVE/stray.caddy"
echo "INVALID" >>"$STAGED/socialos.caddy"
HOST_PROXY_CHECK_URLS="$URLS" install
assert_eq "re-run with a broken staged snippet: exit" 1 "$rc"
assert_eq "re-run, broken: live snippet is the applied one again" "$good_snippet" "$(cat "$LIVE/socialos.caddy")"
assert_eq "re-run, broken: the stray file is gone" "socialos.caddy" "$(live_files)"
assert_eq "re-run, broken: Caddyfile restored" "$good_caddyfile" "$(cat "$SB/etc/Caddyfile")"
assert_eq "re-run, broken: applied record untouched" "$good_snippet" "$(cat "$LIVE/.applied/socialos.caddy")"

# 6. the live set is exactly the staged set: a snippet that is no longer staged is removed (and from the applied record)
setup
echo 'old.example.com {}' >"$LIVE/old.caddy"
mkdir -p "$LIVE/.applied" && chmod 755 "$LIVE/.applied" && cp "$LIVE/old.caddy" "$LIVE/.applied/old.caddy"
HOST_PROXY_CHECK_URLS="$URLS" install
assert_eq "set shrinks: exit" 0 "$rc"
assert_eq "set shrinks: live set equals the staged set" "socialos.caddy" "$(live_files)"
assert_no_file "set shrinks: applied record follows" "$LIVE/.applied/old.caddy"

# 7. the reload fails: Caddy kept its old configuration, everything is put back
setup
touch "$SB/reload.fail"
HOST_PROXY_CHECK_URLS="$URLS" install
assert_eq "reload fails: exit" 1 "$rc"
assert_eq "reload fails: Caddyfile restored" "$orig" "$(cat "$SB/etc/Caddyfile")"
assert_eq "reload fails: no snippet left" "" "$(live_files)"
assert_has "reload fails: reason" "$out" "systemctl reload caddy failed (Caddy kept its previous configuration)"
assert_has "reload fails: Caddy state" "$out" "never loaded the new configuration"
assert_has "reload fails: verdict" "$out" "ROLLED BACK"

# 8. a site stops answering after the reload (503): restored, reloaded again, verified
setup
touch "$SB/flip"
HOST_PROXY_CHECK_URLS="$URLS" install
assert_eq "check fails after reload: exit" 1 "$rc"
assert_eq "check fails after reload: Caddyfile restored" "$orig" "$(cat "$SB/etc/Caddyfile")"
assert_eq "check fails after reload: no snippet left" "" "$(live_files)"
assert_eq "check fails after reload: reloaded twice (apply, restore)" 2 "$(reloads)"
assert_has "check fails after reload: reason" "$out" "no longer answers"
assert_has "check fails after reload: Caddy state" "$out" "Caddy reloaded: it runs the restored configuration"
assert_has "check fails after reload: verified" "$out" "check URL(s) answer"
assert_has "check fails after reload: verdict" "$out" "ROLLED BACK"

# 9. a site breaks AND the reload after the restore fails: the host is NOT known to be fine (exit 4, no false comfort)
setup
touch "$SB/flip"
echo 2 >"$SB/reload.fail-from"
HOST_PROXY_CHECK_URLS="$URLS" install
assert_eq "restore reload fails: exit 4" 4 "$rc"
assert_eq "restore reload fails: files are restored all the same" "$orig" "$(cat "$SB/etc/Caddyfile")"
assert_has "restore reload fails: says Caddy runs the new configuration" "$out" "CADDY IS STILL RUNNING THE NEW CONFIGURATION"
assert_has "restore reload fails: verdict" "$out" "ROLLBACK NOT VERIFIED"
assert_lacks "restore reload fails: no false verdict" "$out" "ROLLED BACK."

# 10. the site is still down after the restore: verified failure, not a success message
setup
touch "$SB/flip-always"
HOST_PROXY_CHECK_URLS="$URLS" install
assert_eq "still failing after the restore: exit 4" 4 "$rc"
assert_has "still failing after the restore: says so" "$out" "STILL FAIL after the restore"
assert_has "still failing after the restore: verdict" "$out" "ROLLBACK NOT VERIFIED"
assert_lacks "still failing after the restore: no false verdict" "$out" "ROLLED BACK."

# 11. interrupted in the middle (SIGTERM during validation): rolled back like any other failure
setup
touch "$SB/slow"
PATH="$SB/bin:$PATH" ALLOW_NON_ROOT=1 SOCIALOS_DIR="$SB/opt-root" CADDYFILE="$SB/etc/Caddyfile" CHECK_TRIES=2 CHECK_DELAY=0 HOST_PROXY_CHECK_URLS="$URLS" \
  bash "$REPO_DEPLOY/host-proxy/install-caddy-import.sh" >"$SB/sig.out" 2>&1 &
pid=$!
sleep 1
kill -TERM "$pid"
rc=0
wait "$pid" || rc=$?
assert_eq "SIGTERM: exit" 1 "$rc"
assert_eq "SIGTERM: Caddyfile restored" "$orig" "$(cat "$SB/etc/Caddyfile")"
assert_eq "SIGTERM: no snippet left" "" "$(live_files)"
assert_has "SIGTERM: reason" "$(cat "$SB/sig.out")" "interrupted by a signal"

# 12. a site is already down before the change: nothing at all is touched, and it says so
setup
echo 502 >"$SB/status"
HOST_PROXY_CHECK_URLS="$URLS" install
assert_eq "unhealthy before: exit" 3 "$rc"
assert_eq "unhealthy before: Caddyfile untouched" "$orig" "$(cat "$SB/etc/Caddyfile")"
assert_eq "unhealthy before: no snippet went live" "" "$(live_files)"
assert_eq "unhealthy before: no backup" 0 "$(backups)"
assert_no_file "unhealthy before: nothing called" "$SB/calls"
assert_has "unhealthy before: truthful" "$out" "Nothing was changed"
echo 000 >"$SB/status"
HOST_PROXY_CHECK_URLS="$URLS" install
assert_eq "no answer at all before: exit" 3 "$rc"
echo 404 >"$SB/status"
HOST_PROXY_CHECK_URLS="$URLS" install
assert_eq "a 404 is still an answer below 500: exit" 0 "$rc"

# 13. URLs: required, validated, and also read from .env
setup
install
assert_eq "no URLs: exit" 2 "$rc"
assert_has "no URLs: explained" "$out" "HOST_PROXY_CHECK_URLS is empty"
assert_eq "no URLs: nothing changed" "$orig" "$(cat "$SB/etc/Caddyfile")"
assert_eq "no URLs: nothing went live" "" "$(live_files)"
HOST_PROXY_CHECK_URLS="ftp://x/ https://ok.example/" install
assert_eq "bad URL: exit" 2 "$rc"
install --no-checks
assert_eq "--no-checks: exit" 0 "$rc"
assert_has "--no-checks: warns" "$out" "no URL checks"
setup
printf 'DOMAIN=example.com\nHOST_PROXY_CHECK_URLS="%s"\n' "$URLS" >"$SB/opt-root/.env"
install
assert_eq "URLs from .env: exit" 0 "$rc"
assert_has "URLs from .env: used" "$out" "checking 2 URL(s)"

# 14. prerequisites: nothing staged; missing Caddyfile
setup
rm "$STAGED/socialos.caddy"
HOST_PROXY_CHECK_URLS="$URLS" install
assert_eq "nothing staged: exit" 2 "$rc"
assert_has "nothing staged: explained" "$out" "render-caddy.sh first"
setup
rm "$SB/etc/Caddyfile"
HOST_PROXY_CHECK_URLS="$URLS" install
assert_eq "no Caddyfile: exit" 2 "$rc"

# 15. the folders whose content becomes host Caddy configuration must not be writable by anyone else
setup
chmod 775 "$LIVE"
HOST_PROXY_CHECK_URLS="$URLS" install
assert_eq "group-writable imported folder: exit" 2 "$rc"
assert_has "group-writable imported folder: explained" "$out" "writable by group or others"
assert_eq "group-writable imported folder: nothing changed" 0 "$(backups)"
setup
chmod 777 "$STAGED"
HOST_PROXY_CHECK_URLS="$URLS" install
assert_eq "world-writable staging folder: exit" 2 "$rc"
setup
chmod 666 "$STAGED/socialos.caddy"
HOST_PROXY_CHECK_URLS="$URLS" install
assert_eq "world-writable staged file: exit" 2 "$rc"
if [ "$(id -u)" -eq 0 ]; then
  setup
  chown 12345 "$LIVE"
  HOST_PROXY_CHECK_URLS="$URLS" install
  assert_eq "folder owned by another user: exit" 2 "$rc"
  assert_has "folder owned by another user: explained" "$out" "do not chown it to a deploy user"
fi

finish
