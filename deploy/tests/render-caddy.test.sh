#!/usr/bin/env bash
# host-proxy/render-caddy.sh: hostnames, ports, the optional s3 site and input validation.
# shellcheck source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

# render ENV_LINES [ARGS]: writes the env file, sets $out (stdout+stderr) and $rc.
render() {
  new_sb
  printf '%s\n' "$1" >"$SB/env"
  shift
  rc=0
  out=$(bash "$REPO_DEPLOY/host-proxy/render-caddy.sh" --env "$SB/env" --out - "$@" 2>&1) || rc=$?
}

render $'DOMAIN=example.com\nCOMPOSE_PROFILES=minio'
assert_eq "defaults: exit" 0 "$rc"
for want in '(socialos_common) {' 'app.example.com {' 'api.example.com {' 'mcp.example.com {' 's3.example.com {' \
  'reverse_proxy 127.0.0.1:13000' 'reverse_proxy 127.0.0.1:18080' 'reverse_proxy 127.0.0.1:13333' 'reverse_proxy 127.0.0.1:19000' \
  'import socialos_common' 'Strict-Transport-Security "max-age=31536000"' 'X-Frame-Options "DENY"' \
  'Content-Security-Policy "upgrade-insecure-requests"' '@metrics path /metrics /metrics/*' 'respond @metrics 404' 'flush_interval -1' \
  'encode zstd gzip'; do
  assert_has "defaults contain: $want" "$out" "$want"
done
assert_eq "flush_interval only for mcp and s3" 2 "$(grep -c 'flush_interval' <<<"$out")"
assert_eq "encode only for app and api" 2 "$(grep -c 'encode zstd gzip' <<<"$out")"
assert_lacks "no template markers left" "$out" '@'"@"
assert_lacks "no unreplaced placeholder" "$out" '@DOMAIN@'
assert_eq "no global options block" 0 "$(grep -c '^{' <<<"$out")"
assert_eq "no snippet called plain 'common'" 0 "$(grep -c '^(common)' <<<"$out")"

render $'DOMAIN=example.com\nFRONTEND_HOST_PORT=14000\nBACKEND_HOST_PORT=18081\nMCP_HOST_PORT=14333\nMINIO_HOST_PORT=19001\nS3_SITE=media.example.com\nCOMPOSE_PROFILES=minio'
assert_has "ports and S3_SITE from .env" "$out" 'reverse_proxy 127.0.0.1:14000'
assert_has "ports: backend" "$out" 'reverse_proxy 127.0.0.1:18081'
assert_has "ports: mcp" "$out" 'reverse_proxy 127.0.0.1:14333'
assert_has "ports: minio" "$out" 'reverse_proxy 127.0.0.1:19001'
assert_has "custom S3_SITE" "$out" 'media.example.com {'

render $'DOMAIN=example.com\nS3_SITE=:8099\nCOMPOSE_PROFILES='
assert_eq "S3_SITE=:8099: exit" 0 "$rc"
assert_lacks "S3_SITE=:8099: no s3 site" "$out" 's3.example.com'
assert_lacks "S3_SITE=:8099: no public port opened" "$out" ':8099'
assert_lacks "S3_SITE=:8099: no minio upstream" "$out" '19000'
assert_has "S3_SITE=:8099: app still there" "$out" 'app.example.com {'

render $'DOMAIN=example.com\nCOMPOSE_PROFILES='
assert_lacks "no minio profile and no S3_SITE: no s3 site" "$out" 's3.example.com'
render $'DOMAIN=example.com\nS3_SITE=s3.example.com\nCOMPOSE_PROFILES='
assert_has "explicit S3_SITE wins over the missing profile" "$out" 's3.example.com {'

render $'DOMAIN="example.com"\nCOMPOSE_PROFILES=other,minio'
assert_has "quoted DOMAIN, minio among profiles" "$out" 's3.example.com {'

# input validation: nothing is written for a bad value
for bad_env in 'DOMAIN=' 'DOMAIN=exa mple.com' 'DOMAIN=a.com}\nimport /etc/passwd' 'DOMAIN=-bad.com' \
  $'DOMAIN=example.com\nFRONTEND_HOST_PORT=abc' $'DOMAIN=example.com\nMCP_HOST_PORT=70000' $'DOMAIN=example.com\nBACKEND_HOST_PORT=0' \
  $'DOMAIN=example.com\nS3_SITE=bad host'; do
  render "$bad_env"
  if [ "$rc" -ne 0 ]; then ok; else bad "rejects: $bad_env" "exit 0"; fi
done
rc=0
out=$(bash "$REPO_DEPLOY/host-proxy/render-caddy.sh" --env "$T_ROOT/does-not-exist" --out - 2>&1) || rc=$?
assert_eq "missing env file: exit" 1 "$rc"

# writing a file: atomic, world-readable (the host's caddy user reads it), same content as stdout, nested dir created
new_sb
printf 'DOMAIN=example.com\nCOMPOSE_PROFILES=minio\n' >"$SB/env"
bash "$REPO_DEPLOY/host-proxy/render-caddy.sh" --env "$SB/env" --out "$SB/new/dir/socialos.caddy" >/dev/null
assert_file "file written" "$SB/new/dir/socialos.caddy"
assert_eq "file mode is 644" 644 "$(stat -c %a "$SB/new/dir/socialos.caddy")"
assert_eq "file equals stdout" "$(bash "$REPO_DEPLOY/host-proxy/render-caddy.sh" --env "$SB/env" --out -)" "$(cat "$SB/new/dir/socialos.caddy")"
assert_eq "no temp files left" "socialos.caddy" "$(ls "$SB/new/dir")"

finish
