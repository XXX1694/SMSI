#!/usr/bin/env bash
# init-env.sh with a stubbed openssl: standalone needs the ACME e-mail, host-proxy mode makes it optional.
# shellcheck source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

setup() {
  new_sb
  mkdir -p "$SB/app"
  cp "$REPO_DEPLOY/init-env.sh" "$REPO_DEPLOY/.env.prod.example" "$SB/app/"
  cat >"$SB/bin/openssl" <<'STUB'
#!/usr/bin/env bash
# openssl rand -hex N | -base64 N
n=$3
case "$2" in
  -hex) head -c "$((n * 2))" < <(yes a | tr -d '\n') ;;
  -base64) printf 'QUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUE=' ;;
esac
STUB
  chmod +x "$SB/bin/openssl"
}
run() { # run ARGS: sets $out and $rc
  rc=0
  out=$(cd "$SB/app" && PATH="$SB/bin:$PATH" bash ./init-env.sh "$@" 2>&1) || rc=$?
}
value() { grep "^$1=" "$SB/app/.env" | tail -n 1 | cut -d= -f2-; }
placeholders() { grep -cE '^[A-Za-z_][A-Za-z0-9_]*=.*CHANGE_ME' "$SB/app/.env" || true; }

setup
run example.com ops@example.com
assert_eq "standalone: exit" 0 "$rc"
assert_eq "standalone: DOMAIN" example.com "$(value DOMAIN)"
assert_eq "standalone: ACME_EMAIL" ops@example.com "$(value ACME_EMAIL)"
assert_eq "standalone: compose file unchanged" docker-compose.prod.yml "$(value COMPOSE_FILE)"
assert_eq "standalone: no placeholders left" 0 "$(placeholders)"
assert_eq "standalone: mode 600" 600 "$(stat -c %a "$SB/app/.env")"

setup
run example.com
assert_eq "standalone without e-mail: usage error" 2 "$rc"
assert_has "standalone without e-mail: usage" "$out" "usage:"
assert_no_file "standalone without e-mail: no .env" "$SB/app/.env"

setup
run --host-proxy example.com
assert_eq "host-proxy without e-mail: exit" 0 "$rc"
assert_eq "host-proxy without e-mail: ACME_EMAIL is empty" "" "$(value ACME_EMAIL)"
assert_eq "host-proxy: both compose files" docker-compose.prod.yml:docker-compose.host-proxy.yml "$(value COMPOSE_FILE)"
assert_eq "host-proxy without e-mail: no placeholders left" 0 "$(placeholders)"
assert_has "host-proxy: next steps" "$out" "render-caddy.sh"

setup
run --host-proxy example.com ops@example.com
assert_eq "host-proxy with e-mail: exit" 0 "$rc"
assert_eq "host-proxy with e-mail: ACME_EMAIL" ops@example.com "$(value ACME_EMAIL)"

for args_case in "bad_domain! ops@example.com" "example.com not-an-email" "--host-proxy example.com not-an-email" "--host-proxy bad_domain!" ""; do
  setup
  # shellcheck disable=SC2086 # the word splitting is the point: each case is a list of arguments
  run $args_case
  if [ "$rc" -ne 0 ]; then ok; else bad "rejects: '$args_case'" "exit 0"; fi
  assert_no_file "rejects '$args_case': no .env written" "$SB/app/.env"
done

setup
echo "KEEP=1" >"$SB/app/.env"
run example.com ops@example.com
assert_eq "existing .env: refused" 1 "$rc"
assert_eq "existing .env: untouched" "KEEP=1" "$(cat "$SB/app/.env")"

finish
