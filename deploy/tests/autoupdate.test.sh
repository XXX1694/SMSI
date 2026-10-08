#!/usr/bin/env bash
# autoupdate.sh against a stubbed GitHub API (curl) and a stubbed deploy.sh.
# shellcheck source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

# setup [ENV_LINES]: sandbox with autoupdate.sh, a stub deploy.sh (records its arguments, exits with the code in
# $SB/deploy.rc) and a stub curl (answers with $SB/api.code and $SB/api.json, records the URL).
setup() {
  new_sb
  mkdir -p "$SB/app"
  cp "$REPO_DEPLOY/autoupdate.sh" "$SB/app/"
  printf '%s\n' "${1-}" >"$SB/app/.env"
  echo 0 >"$SB/deploy.rc"
  echo 200 >"$SB/api.code"
  api_release v1.2.3
  cat >"$SB/app/deploy.sh" <<STUB
#!/usr/bin/env bash
echo "\$*" >>"$SB/deploy.calls"
exit "\$(cat "$SB/deploy.rc")"
STUB
  cat >"$SB/bin/curl" <<STUB
#!/usr/bin/env bash
out=""
while [ \$# -gt 0 ]; do
  case "\$1" in -o) out=\$2; shift ;; http*) echo "\$1" >>"$SB/curl.calls" ;; esac
  shift
done
cp "$SB/api.json" "\$out"
cat "$SB/api.code"
STUB
  chmod +x "$SB/app/deploy.sh" "$SB/bin/curl"
}
api_release() { printf '{\n  "url": "x",\n  "author": {"login": "someone"},\n  "tag_name": "%s",\n  "name": "release",\n  "body": "\\"tag_name\\": \\"v9.9.9\\""\n}\n' "$1" >"$SB/api.json"; }
run() { # run [ARGS]: sets $out and $rc
  rc=0
  out=$(cd "$SB/app" && PATH="$SB/bin:$PATH" bash ./autoupdate.sh "$@" 2>&1) || rc=$?
}
calls() { cat "$SB/deploy.calls" 2>/dev/null || true; }

# 1. a new release is deployed, under the image tag without the "v"
setup
run
assert_eq "new release: exit" 0 "$rc"
assert_eq "new release: deploy.sh gets 1.2.3" "1.2.3" "$(calls)"
assert_has "new release: logged" "$out" "deployed v1.2.3"
assert_has "default repo is queried" "$(cat "$SB/curl.calls")" "/repos/XXX1694/SMSI/releases/latest"

# 2. nothing to do when the deployed tag is current
setup
mkdir -p "$SB/app/.deploy" && echo 1.2.3 >"$SB/app/.deploy/current_tag"
run
assert_eq "up to date: exit" 0 "$rc"
assert_eq "up to date: no deploy" "" "$(calls)"
assert_has "up to date: logged" "$out" "up to date"

# 3. only vX.Y.Z is accepted
for odd in v1.2.3-rc.1 latest 1.2.3 v1.2 v1.2.3.4 'v1.2.3; reboot'; do
  setup
  api_release "$odd"
  run
  assert_eq "tag '$odd': exit" 0 "$rc"
  assert_eq "tag '$odd': no deploy" "" "$(calls)"
  assert_has "tag '$odd': explained" "$out" "not a vX.Y.Z tag"
done

# 4. opt-out
for off in false FALSE off 0 no; do
  setup "AUTOUPDATE=$off"
  run
  assert_eq "AUTOUPDATE=$off: exit" 0 "$rc"
  assert_eq "AUTOUPDATE=$off: no deploy" "" "$(calls)"
  assert_no_file "AUTOUPDATE=$off: GitHub not asked" "$SB/curl.calls"
done
setup "AUTOUPDATE=true"
run
assert_eq "AUTOUPDATE=true deploys" "1.2.3" "$(calls)"

# 5. GITHUB_REPO from .env; a malformed one is refused
setup 'GITHUB_REPO="acme/widgets"'
run
assert_has "GITHUB_REPO is used" "$(cat "$SB/curl.calls")" "/repos/acme/widgets/releases/latest"
setup "GITHUB_REPO=not a repo"
run
assert_eq "bad GITHUB_REPO: exit" 1 "$rc"
assert_eq "bad GITHUB_REPO: no deploy" "" "$(calls)"

# 6. a failed deploy is remembered and not retried, until a newer release appears
setup
echo 1 >"$SB/deploy.rc"
run
assert_eq "failed deploy: exit" 1 "$rc"
assert_has "failed deploy: marker" "$(cat "$SB/app/.deploy/autoupdate_failed")" "1.2.3 "
echo 0 >"$SB/deploy.rc"
run
assert_eq "after failure: exit" 0 "$rc"
assert_eq "after failure: not retried" "1.2.3" "$(calls)"
assert_has "after failure: explained" "$out" "skipping v1.2.3"
api_release v1.2.4
run
assert_eq "newer release is tried" "$(printf '1.2.3\n1.2.4')" "$(calls)"
assert_no_file "success clears the marker" "$SB/app/.deploy/autoupdate_failed"

# 7. "nothing was changed" (75) is retried later, not recorded as a failure
setup
echo 75 >"$SB/deploy.rc"
run
assert_eq "exit 75: exit" 0 "$rc"
assert_no_file "exit 75: no failure marker" "$SB/app/.deploy/autoupdate_failed"
assert_file "exit 75: retry marker" "$SB/app/.deploy/autoupdate_retry"
run
assert_eq "exit 75: backoff, one call only" "1.2.3" "$(calls)"
assert_has "exit 75: backoff logged" "$out" "waiting to retry"
echo 0 >"$SB/deploy.rc"
echo "1.2.3 $(($(date +%s) - 1))" >"$SB/app/.deploy/autoupdate_retry" # the delay is over
run
assert_eq "exit 75: retried after the delay" "$(printf '1.2.3\n1.2.3')" "$(calls)"
assert_no_file "exit 75: retry marker cleared on success" "$SB/app/.deploy/autoupdate_retry"

# 8. GitHub trouble is not a failure of this host and never deploys
for code in 404 403 500 000; do
  setup
  echo "$code" >"$SB/api.code"
  run
  assert_eq "HTTP $code: exit" 0 "$rc"
  assert_eq "HTTP $code: no deploy" "" "$(calls)"
done

# 9. overlapping runs: the second one backs off
setup
mkdir -p "$SB/app/.deploy"
flock "$SB/app/.deploy/autoupdate.lock" sleep 4 &
holder=$!
sleep 1
run
kill "$holder" 2>/dev/null || true
wait "$holder" 2>/dev/null || true
assert_eq "overlap: exit" 0 "$rc"
assert_eq "overlap: no deploy" "" "$(calls)"
assert_has "overlap: logged" "$out" "still working"

# 10. --dry-run changes nothing
setup
run --dry-run
assert_eq "dry run: exit" 0 "$rc"
assert_eq "dry run: no deploy" "" "$(calls)"
assert_has "dry run: says what it would do" "$out" "would deploy v1.2.3 as image tag 1.2.3"

finish
