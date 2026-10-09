#!/usr/bin/env bash
# autoupdate.sh against a stubbed GitHub API (curl) and a stubbed deploy.sh.
# shellcheck source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

# setup [ENV_LINES]: sandbox with autoupdate.sh, a stub deploy.sh (records its arguments, exits with the code in
# $SB/deploy.rc) and a stub curl (GitHub API and ghcr.io answers from files in $SB, records every URL).
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
  cat >"$SB/bin/curl" <<'STUB'
#!/usr/bin/env bash
# Routes by URL: the GitHub API ($STUB_DIR/api.{json,code}), the ghcr.io token endpoint (token.<image>.{json,code}) and
# the ghcr.io manifest HEAD (manifest.<image>.code). Unset files mean "fine": a token and 200.
out=""
url=""
follow=0
echo "$*" >>"$STUB_DIR/curl.argv"
while [ $# -gt 0 ]; do
  case "$1" in -o) out=$2; shift ;; http*) url=$1 ;; -L | --location | -[a-zA-Z]*L*) follow=1 ;; esac
  shift
done
echo "$url" >>"$STUB_DIR/curl.calls"
body=""
case "$url" in
  *api.github.com*)
    body=$(cat "$STUB_DIR/api.json")
    code=$(cat "$STUB_DIR/api.code")
    # api.redirect: the repository was renamed, so the API answers 301 unless the client follows redirects (-L).
    if [ -e "$STUB_DIR/api.redirect" ] && [ "$follow" = 0 ]; then code=301; body=""; fi
    ;;
  *ghcr.io/token*)
    name=${url##*socialos-}
    name=${name%%:*}
    code=200
    [ ! -e "$STUB_DIR/token.$name.code" ] || code=$(cat "$STUB_DIR/token.$name.code")
    body="{\"token\":\"tok-$name\"}"
    [ ! -e "$STUB_DIR/token.$name.json" ] || body=$(cat "$STUB_DIR/token.$name.json")
    ;;
  *ghcr.io/v2/*)
    name=${url#*socialos-}
    name=${name%%/*}
    code=200
    [ ! -e "$STUB_DIR/manifest.$name.code" ] || code=$(cat "$STUB_DIR/manifest.$name.code")
    ;;
  *)
    code=000
    ;;
esac
if [ -n "$out" ] && [ "$out" != /dev/null ]; then printf '%s' "$body" >"$out"; fi
printf '%s' "$code"
STUB
  chmod +x "$SB/app/deploy.sh" "$SB/bin/curl"
}
api_release() { printf '{\n  "url": "x",\n  "author": {"login": "someone"},\n  "tag_name": "%s",\n  "name": "release",\n  "body": "\\"tag_name\\": \\"v9.9.9\\""\n}\n' "$1" >"$SB/api.json"; }
run() { # run [ARGS]: sets $out and $rc
  rc=0
  out=$(cd "$SB/app" && STUB_DIR="$SB" PATH="$SB/bin:$PATH" bash ./autoupdate.sh "$@" 2>&1) || rc=$?
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

# 4b. the host guard has shed load (host under pressure): no deploy, GitHub not even asked; deploys again once it is gone
setup
mkdir -p "$SB/app/.deploy/guard" && echo 2 >"$SB/app/.deploy/guard/shed"
run
assert_eq "guard shed: exit" 0 "$rc"
assert_eq "guard shed: no deploy" "" "$(calls)"
assert_no_file "guard shed: GitHub not asked" "$SB/curl.calls"
assert_has "guard shed: explained" "$out" "host guard has shed SocialOS load (level 2)"
rm "$SB/app/.deploy/guard/shed"
run
assert_eq "guard resumed: deploys" "1.2.3" "$(calls)"

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

# 8b. after a repository rename the API answers 301: the redirect is followed (curl -L, https only, bounded)
setup
touch "$SB/api.redirect"
run
assert_eq "301 then 200: exit" 0 "$rc"
assert_eq "301 then 200: deployed" "1.2.3" "$(calls)"
assert_has "301 then 200: curl follows redirects" "$(grep api.github.com "$SB/curl.argv")" "-L --proto-redir =https --max-redirs 3"

# 8c. a final non-200 (404 after the redirects) is logged as a warning, not swallowed; the run still ends cleanly
for code in 404 301 500; do
  setup
  echo "$code" >"$SB/api.code"
  out=$(cd "$SB/app" && STUB_DIR="$SB" PATH="$SB/bin:$PATH" bash ./autoupdate.sh 2>&1 >/dev/null) && rc=0 || rc=$?
  assert_eq "HTTP $code: warning exit" 0 "$rc"
  assert_has "HTTP $code: warning on stderr" "$out" "autoupdate: "
  assert_has "HTTP $code: names the repo" "$out" "XXX1694/SMSI"
done

# 8d. GITHUB_REPO from the process environment wins over the default
setup
(cd "$SB/app" && GITHUB_REPO=acme/renamed STUB_DIR="$SB" PATH="$SB/bin:$PATH" bash ./autoupdate.sh >/dev/null 2>&1) || true
assert_has "GITHUB_REPO env is used" "$(cat "$SB/curl.calls")" "/repos/acme/renamed/releases/latest"

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

# 11. only a strictly newer release is deployed: numeric, field by field, no downgrades, nothing sideways
deployed_then_latest() { # deployed_then_latest DEPLOYED LATEST: sets $out, $rc; the stub deploy.sh records its calls
  setup
  mkdir -p "$SB/app/.deploy" && echo "$1" >"$SB/app/.deploy/current_tag"
  api_release "$2"
  run
}
for pair in "1.2.3 v1.2.4" "1.2.3 v1.3.0" "1.2.3 v2.0.0" "1.9.0 v1.10.0" "1.2.3 v1.2.10" "0.9.9 v1.0.0" "v1.2.3 v1.2.4" "1.0.0 v99999999999999999999.0.0"; do
  read -r have want <<<"$pair"
  deployed_then_latest "$have" "$want"
  assert_eq "newer ($have -> $want): exit" 0 "$rc"
  assert_eq "newer ($have -> $want): deployed" "${want#v}" "$(calls)"
done
for pair in "1.2.4 v1.2.3" "2.0.0 v1.9.9" "1.10.0 v1.9.0" "1.2.10 v1.2.9" "1.0.0 v0.9.9" "1.2.3 v1.2.03" "99999999999999999999.0.0 v1.0.0" "v1.2.4 v1.2.3"; do
  read -r have want <<<"$pair"
  deployed_then_latest "$have" "$want"
  assert_eq "not newer ($have -> $want): exit" 0 "$rc"
  assert_eq "not newer ($have -> $want): not deployed" "" "$(calls)"
  assert_has "not newer ($have -> $want): logged" "$out" "refusing $want: it is not newer than the deployed"
done
for have in sha-1a2b3c4 main v1.2 1.2.3-rc.1; do
  deployed_then_latest "$have" v9.9.9
  assert_eq "deployed '$have' is not a release: exit" 0 "$rc"
  assert_eq "deployed '$have' is not a release: not deployed" "" "$(calls)"
  assert_has "deployed '$have' is not a release: explained" "$out" "is not a release version"
done
# a refused downgrade is not recorded as a failed deploy
deployed_then_latest 2.0.0 v1.9.9
assert_no_file "downgrade is not a failure" "$SB/app/.deploy/autoupdate_failed"
assert_no_file "downgrade leaves no retry marker" "$SB/app/.deploy/autoupdate_retry"

# 12. the images must exist in ghcr.io before deploy.sh is called; "not there yet" is neither a failure nor a retry marker
ghcr_calls() { grep -c 'ghcr.io' "$SB/curl.calls" || true; }
markers() { # how many of the failed/retry markers exist
  local n=0 f
  for f in "$SB/app/.deploy/autoupdate_failed" "$SB/app/.deploy/autoupdate_retry"; do
    if [ -e "$f" ]; then n=$((n + 1)); fi
  done
  echo "$n"
}

setup # all three present: asked anonymously, one token and one HEAD per image, for the image tag without the "v"
run
assert_eq "images ready: deployed" "1.2.3" "$(calls)"
for name in backend mcp frontend; do
  assert_has "token requested for $name" "$(cat "$SB/curl.calls")" "https://ghcr.io/token?service=ghcr.io&scope=repository:xxx1694/socialos-$name:pull"
  assert_has "manifest of $name asked at 1.2.3" "$(cat "$SB/curl.calls")" "https://ghcr.io/v2/xxx1694/socialos-$name/manifests/1.2.3"
  assert_has "manifest HEAD of $name carries that image's token" "$(cat "$SB/curl.argv")" "Authorization: Bearer tok-$name"
done
assert_has "the manifest check is a HEAD" "$(grep 'manifests' "$SB/curl.argv" | head -n 1)" "-I"

for missing in backend mcp frontend; do
  setup
  echo 404 >"$SB/manifest.$missing.code"
  run
  assert_eq "$missing missing: exit" 0 "$rc"
  assert_eq "$missing missing: deploy.sh not called" "" "$(calls)"
  assert_has "$missing missing: logged" "$out" "images not ready"
  assert_has "$missing missing: names the image" "$out" "socialos-{$missing}:1.2.3"
  assert_eq "$missing missing: nothing recorded (no failed, no retry marker)" 0 "$(markers)"
done
setup # the next tick: the images are there now
echo 404 >"$SB/manifest.mcp.code"
run
assert_eq "retry: first tick, no deploy" "" "$(calls)"
rm "$SB/manifest.mcp.code"
run
assert_eq "retry: next tick deploys" "1.2.3" "$(calls)"
assert_eq "retry: nothing recorded" 0 "$(markers)"

setup # private or unknown package: not visible anonymously
echo 403 >"$SB/token.backend.code"
echo '{"errors":[{"code":"DENIED"}]}' >"$SB/token.backend.json"
run
assert_eq "denied: exit" 0 "$rc"
assert_eq "denied: deploy.sh not called" "" "$(calls)"
assert_has "denied: logged" "$out" "images not ready"
assert_has "denied: hint for private packages" "$out" "AUTOUPDATE_CHECK_IMAGES=false"
assert_eq "denied: nothing recorded" 0 "$(markers)"

setup # registry trouble: not a verdict about the images
echo 503 >"$SB/manifest.frontend.code"
run
assert_eq "registry error: exit" 0 "$rc"
assert_eq "registry error: deploy.sh not called" "" "$(calls)"
assert_has "registry error: logged" "$out" "could not check ghcr.io for frontend"
assert_eq "registry error: nothing recorded" 0 "$(markers)"
setup
echo 500 >"$SB/token.mcp.code"
run
assert_has "token endpoint error: logged" "$out" "could not check ghcr.io for mcp"
assert_eq "token endpoint error: deploy.sh not called" "" "$(calls)"

setup 'AUTOUPDATE_CHECK_IMAGES=false'
echo 404 >"$SB/manifest.backend.code"
run
assert_eq "check switched off: deploys anyway" "1.2.3" "$(calls)"
assert_eq "check switched off: ghcr.io not asked" 0 "$(ghcr_calls)"

setup 'GHCR_OWNER=Acme-Corp'
run
assert_has "owner from .env, lower-cased" "$(cat "$SB/curl.calls")" "/v2/acme-corp/socialos-backend/manifests/1.2.3"

setup # --dry-run reports the missing images too
echo 404 >"$SB/manifest.mcp.code"
run --dry-run
assert_has "dry run: images not ready" "$out" "images not ready"
assert_lacks "dry run: does not claim it would deploy" "$out" "would deploy"

# ghcr.io is only asked for a release that would be deployed: not for refused, up-to-date, failed or backing-off tags
setup
mkdir -p "$SB/app/.deploy" && echo 2.0.0 >"$SB/app/.deploy/current_tag"
run
assert_eq "refused downgrade: ghcr.io not asked" 0 "$(ghcr_calls)"
setup
mkdir -p "$SB/app/.deploy" && echo 1.2.3 >"$SB/app/.deploy/current_tag"
run
assert_eq "up to date: ghcr.io not asked" 0 "$(ghcr_calls)"
setup
mkdir -p "$SB/app/.deploy" && echo "1.2.3 2026-10-09T00:00:00Z" >"$SB/app/.deploy/autoupdate_failed"
run
assert_eq "failed earlier: ghcr.io not asked" 0 "$(ghcr_calls)"

finish
