#!/usr/bin/env bash
# host-proxy/install-caddy-import.sh against stubbed caddy, systemctl and curl: every failure restores the old state.
# shellcheck source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

# setup: sandbox "host" with a Caddyfile, a rendered snippet and stubs.
#   caddy validate  fails when the Caddyfile or an imported snippet contains the word INVALID
#   systemctl       records the call; fails when $SB/reload.fail exists; sets the check URL to 503 when $SB/flip exists
#   curl            prints the status in $SB/status (200 unless a test changes it)
setup() {
  new_sb
  mkdir -p "$SB/etc" "$SB/opt-root/caddy"
  printf '{\n\temail ops@example.com\n}\n\nexample.org {\n\trespond "other site"\n}\n' >"$SB/etc/Caddyfile"
  printf 'app.example.com {\n\trespond "ok"\n}\n' >"$SB/opt-root/caddy/socialos.caddy"
  echo 200 >"$SB/status"
  cat >"$SB/bin/caddy" <<STUB
#!/usr/bin/env bash
echo "caddy \$*" >>"$SB/calls"
if grep -q INVALID "$SB/etc/Caddyfile" "$SB"/opt-root/caddy/*.caddy 2>/dev/null; then echo "Error: adapting config: INVALID" >&2; exit 1; fi
STUB
  cat >"$SB/bin/systemctl" <<STUB
#!/usr/bin/env bash
echo "systemctl \$*" >>"$SB/calls"
[ -e "$SB/reload.fail" ] && exit 1
[ -e "$SB/flip" ] && echo 503 >"$SB/status"
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
imports() { grep -c "^import $SB/opt-root/caddy/\*.caddy\$" "$SB/etc/Caddyfile" || true; }
reloads() { grep -c '^systemctl reload caddy' "$SB/calls" || true; }
backups() {
  local n=0 f
  for f in "$SB"/etc/Caddyfile.socialos-backup-*; do
    if [ -e "$f" ]; then n=$((n + 1)); fi
  done
  echo "$n"
}
URLS="https://www.example.org/ https://example.net/health"

# 1. first install: backup, import line, validate with the right arguments, reload, checks, snapshot
setup
HOST_PROXY_CHECK_URLS="$URLS" install
assert_eq "first install: exit" 0 "$rc"
assert_eq "import line appended once" 1 "$(imports)"
assert_eq "one timestamped backup" 1 "$(backups)"
assert_eq "backup equals the original" "$orig" "$(cat "$SB"/etc/Caddyfile.socialos-backup-*)"
assert_has "validated the right way" "$(cat "$SB/calls")" "caddy validate --config $SB/etc/Caddyfile --adapter caddyfile"
assert_eq "reloaded once" 1 "$(reloads)"
assert_file "last applied snippet saved" "$SB/opt-root/caddy/.applied/socialos.caddy"
assert_eq "the original content is untouched above the import" "$orig" "$(head -n "$(wc -l <<<"$orig")" "$SB/etc/Caddyfile")"

# 2. idempotent: second run adds no second import line
HOST_PROXY_CHECK_URLS="$URLS" install
assert_eq "second run: exit" 0 "$rc"
assert_eq "second run: still one import line" 1 "$(imports)"
assert_has "second run: says so" "$out" "already in"

# 3. a file without a trailing newline and an indented import line already present
setup
printf 'example.org {\n\trespond "x"\n}' >"$SB/etc/Caddyfile"
HOST_PROXY_CHECK_URLS="$URLS" install
assert_eq "no trailing newline: exit" 0 "$rc"
assert_eq "no trailing newline: import on its own line" 1 "$(imports)"
setup
printf 'example.org {\n}\n\t%s  \n' "import $SB/opt-root/caddy/*.caddy" >"$SB/etc/Caddyfile"
HOST_PROXY_CHECK_URLS="$URLS" install
assert_eq "existing indented import: exit" 0 "$rc"
assert_eq "existing indented import: not added again" 0 "$(grep -c '^import' "$SB/etc/Caddyfile")"

# 4. validation fails: Caddyfile restored byte for byte, reloaded, non-zero
setup
echo "INVALID" >>"$SB/opt-root/caddy/socialos.caddy"
HOST_PROXY_CHECK_URLS="$URLS" install
assert_eq "invalid config: exit" 1 "$rc"
assert_eq "invalid config: Caddyfile restored" "$orig" "$(cat "$SB/etc/Caddyfile")"
assert_has "invalid config: validator output shown" "$out" "INVALID"
assert_eq "invalid config: reload only after the restore" 1 "$(reloads)"
assert_no_file "invalid config: nothing recorded as applied" "$SB/opt-root/caddy/.applied/socialos.caddy"

# 5. validation fails on a re-run: the previous good snippet comes back too
setup
HOST_PROXY_CHECK_URLS="$URLS" install
good_snippet=$(cat "$SB/opt-root/caddy/socialos.caddy")
good_caddyfile=$(cat "$SB/etc/Caddyfile")
echo "INVALID" >>"$SB/opt-root/caddy/socialos.caddy"
HOST_PROXY_CHECK_URLS="$URLS" install
assert_eq "re-run with a broken snippet: exit" 1 "$rc"
assert_eq "re-run with a broken snippet: snippet restored" "$good_snippet" "$(cat "$SB/opt-root/caddy/socialos.caddy")"
assert_eq "re-run with a broken snippet: Caddyfile restored" "$good_caddyfile" "$(cat "$SB/etc/Caddyfile")"

# 6. the reload fails
setup
touch "$SB/reload.fail"
HOST_PROXY_CHECK_URLS="$URLS" install
assert_eq "reload fails: exit" 1 "$rc"
assert_eq "reload fails: Caddyfile restored" "$orig" "$(cat "$SB/etc/Caddyfile")"
assert_has "reload fails: reason" "$out" "systemctl reload caddy failed"

# 7. a site stops answering after the reload (503): restored, reloaded again
setup
touch "$SB/flip"
HOST_PROXY_CHECK_URLS="$URLS" install
assert_eq "check fails after reload: exit" 1 "$rc"
assert_eq "check fails after reload: Caddyfile restored" "$orig" "$(cat "$SB/etc/Caddyfile")"
assert_eq "check fails after reload: reloaded twice (apply, restore)" 2 "$(reloads)"
assert_has "check fails after reload: reason" "$out" "no longer answers"

# 8. a site is already down before the change: nothing is touched
setup
echo 502 >"$SB/status"
HOST_PROXY_CHECK_URLS="$URLS" install
assert_eq "unhealthy before: exit" 3 "$rc"
assert_eq "unhealthy before: Caddyfile untouched" "$orig" "$(cat "$SB/etc/Caddyfile")"
assert_eq "unhealthy before: no backup" 0 "$(backups)"
assert_no_file "unhealthy before: nothing called" "$SB/calls"
echo 000 >"$SB/status"
HOST_PROXY_CHECK_URLS="$URLS" install
assert_eq "no answer at all before: exit" 3 "$rc"
echo 404 >"$SB/status"
HOST_PROXY_CHECK_URLS="$URLS" install
assert_eq "a 404 is still an answer below 500: exit" 0 "$rc"

# 9. URLs: required, validated, and also read from .env
setup
install
assert_eq "no URLs: exit" 2 "$rc"
assert_has "no URLs: explained" "$out" "HOST_PROXY_CHECK_URLS is empty"
assert_eq "no URLs: nothing changed" "$orig" "$(cat "$SB/etc/Caddyfile")"
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

# 10. prerequisites: nothing rendered yet; missing Caddyfile
setup
rm "$SB/opt-root/caddy/socialos.caddy"
HOST_PROXY_CHECK_URLS="$URLS" install
assert_eq "no snippet: exit" 2 "$rc"
assert_has "no snippet: explained" "$out" "render-caddy.sh first"
setup
rm "$SB/etc/Caddyfile"
HOST_PROXY_CHECK_URLS="$URLS" install
assert_eq "no Caddyfile: exit" 2 "$rc"

finish
