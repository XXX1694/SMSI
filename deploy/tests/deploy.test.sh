#!/usr/bin/env bash
# deploy.sh with a stubbed `docker`: which compose files it uses, host-proxy mode (no caddy service), exit codes, rollback.
# shellcheck source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

# setup [EXTRA_ENV_LINES] [SERVICES]: sandbox with deploy.sh, a valid .env and a stub docker.
#   docker compose version / config -q    succeed
#   docker compose config --services      prints $SB/services (default: the standalone service list)
#   docker compose pull                   fails when $SB/pull.fail exists
#   docker compose up                     fails when its IMAGE_TAG is the one in $SB/up.fail
#   everything else                       succeeds; every call is recorded with COMPOSE_FILE and IMAGE_TAG
setup() {
  new_sb
  mkdir -p "$SB/app"
  cp "$REPO_DEPLOY/deploy.sh" "$SB/app/"
  printf 'DOMAIN=example.com\nACME_EMAIL=ops@example.com\nIMAGE_TAG=main\n%s\n' "${1-}" >"$SB/app/.env"
  tr ' ' '\n' <<<"${2-postgres redis migrate backend worker mcp frontend caddy}" >"$SB/services"
  cat >"$SB/bin/docker" <<STUB
#!/usr/bin/env bash
echo "COMPOSE_FILE=\${COMPOSE_FILE-unset} IMAGE_TAG=\${IMAGE_TAG-unset} :: \$*" >>"$SB/calls"
case "\$*" in
  "compose config --services") cat "$SB/services" ;;
  "compose pull"*) [ -e "$SB/pull.fail" ] && exit 1 ;;
  "compose up"*) [ -e "$SB/up.fail" ] && [ "\${IMAGE_TAG-}" = "\$(cat "$SB/up.fail")" ] && exit 1 ;;
  "image ls"*) ;;
esac
exit 0
STUB
  chmod +x "$SB/bin/docker"
}
run() { # run [ARGS]: sets $out and $rc
  rc=0
  out=$(cd "$SB/app" && PATH="$SB/bin:$PATH" SKIP_PUBLIC_CHECK=1 bash ./deploy.sh "$@" 2>&1) || rc=$?
}
calls() { cat "$SB/calls" 2>/dev/null || true; }

# 1. host-proxy mode: the compose files from .env reach every docker compose call
setup 'COMPOSE_FILE=docker-compose.prod.yml:docker-compose.host-proxy.yml' 'postgres redis migrate backend worker mcp frontend'
run 1.0.0
assert_eq "host-proxy deploy: exit" 0 "$rc"
assert_eq "host-proxy deploy: every compose call uses both files" 0 \
  "$(calls | grep ' :: compose ' | grep -v ' :: compose version' | grep -vc 'COMPOSE_FILE=docker-compose.prod.yml:docker-compose.host-proxy.yml' || true)"
assert_eq "state: current tag" 1.0.0 "$(cat "$SB/app/.deploy/current_tag")"
assert_has ".env points at the new tag" "$(cat "$SB/app/.env")" "IMAGE_TAG=1.0.0"
assert_has "history" "$(cat "$SB/app/.deploy/history.log")" "1.0.0 ok"
assert_has "pulled the app images only" "$(calls)" ":: compose pull backend worker migrate mcp frontend"
assert_has "ran the migrations" "$(calls)" ":: compose run --rm -T migrate"
assert_has "waited for readiness inside the network" "$(calls)" ":: compose exec -T backend /app/api healthcheck"

# 2. standalone: no COMPOSE_FILE anywhere -> the default file, as before
setup
run 1.0.0
assert_eq "standalone deploy: exit" 0 "$rc"
assert_eq "standalone: default compose file" 0 \
  "$(calls | grep ' :: compose ' | grep -v ' :: compose version' | grep -vc 'COMPOSE_FILE=docker-compose.prod.yml ' || true)"

# 3. COMPOSE_FILE from the environment wins over .env, like for docker compose itself
setup 'COMPOSE_FILE=docker-compose.prod.yml'
rc=0
out=$(cd "$SB/app" && COMPOSE_FILE=other.yml PATH="$SB/bin:$PATH" SKIP_PUBLIC_CHECK=1 bash ./deploy.sh --status 2>&1) || rc=$?
assert_has "status shows the compose files" "$out" "compose files: other.yml"

# 4. pull fails: nothing changed, exit 75, state untouched, no restart
setup
touch "$SB/pull.fail"
run 1.0.0
assert_eq "pull fails: exit 75" 75 "$rc"
assert_no_file "pull fails: no current tag" "$SB/app/.deploy/current_tag"
assert_lacks "pull fails: nothing restarted" "$(calls)" ":: compose up"
assert_has "pull fails: explained" "$out" "nothing was changed"

# 5. another deploy holds the lock: exit 75
setup
mkdir -p "$SB/app/.deploy"
flock "$SB/app/.deploy/lock" sleep 4 &
holder=$!
sleep 1
run 1.0.0
kill "$holder" 2>/dev/null || true
wait "$holder" 2>/dev/null || true
assert_eq "locked: exit 75" 75 "$rc"
assert_has "locked: explained" "$out" "another deploy is already running"
assert_lacks "locked: nothing pulled" "$(calls)" ":: compose pull"

# 6. the new containers fail: rolled back to the previous tag, exit 1; logs are asked from caddy only when it exists
setup
mkdir -p "$SB/app/.deploy" && echo 0.9.0 >"$SB/app/.deploy/current_tag"
echo 1.0.0 >"$SB/up.fail"
run 1.0.0
assert_eq "failed deploy: exit 1" 1 "$rc"
assert_has "failed deploy: rolled back" "$(calls)" "IMAGE_TAG=0.9.0 :: compose up -d --remove-orphans"
assert_eq "failed deploy: current tag unchanged" 0.9.0 "$(cat "$SB/app/.deploy/current_tag")"
assert_has "failed deploy: standalone asks for the caddy logs" "$(calls)" "mcp frontend caddy"
setup '' 'postgres redis migrate backend worker mcp frontend'
mkdir -p "$SB/app/.deploy" && echo 0.9.0 >"$SB/app/.deploy/current_tag"
echo 1.0.0 >"$SB/up.fail"
run 1.0.0
assert_eq "failed host-proxy deploy: exit 1" 1 "$rc"
assert_has "failed host-proxy deploy: logs of the app services" "$(calls)" "logs --no-color --tail=60 migrate backend worker mcp frontend"
assert_lacks "failed host-proxy deploy: no caddy logs requested" "$(calls | grep ' logs ')" "caddy"

# 7. --status works without a caddy container
setup '' 'postgres redis migrate backend worker mcp frontend'
run --status
assert_eq "status: exit" 0 "$rc"
assert_has "status: compose files" "$out" "compose files: docker-compose.prod.yml"

# 8. ACME_EMAIL belongs to the bundled Caddy: required there, not in host-proxy mode (no caddy service)
NOCADDY='postgres redis migrate backend worker mcp frontend'
setup 'ACME_EMAIL='
run 1.0.0
assert_eq "standalone, empty ACME_EMAIL: refused" 1 "$rc"
assert_has "standalone, empty ACME_EMAIL: explained" "$out" "ACME_EMAIL is empty"
assert_lacks "standalone, empty ACME_EMAIL: nothing pulled" "$(calls)" ":: compose pull"
setup 'ACME_EMAIL=CHANGE_ME_you@example.com'
run 1.0.0
assert_eq "standalone, placeholder ACME_EMAIL: refused" 1 "$rc"
assert_has "standalone, placeholder ACME_EMAIL: listed" "$out" "ACME_EMAIL=..."
setup 'ACME_EMAIL=' "$NOCADDY"
run 1.0.0
assert_eq "host-proxy, empty ACME_EMAIL: deploys" 0 "$rc"
setup 'ACME_EMAIL=CHANGE_ME_you@example.com' "$NOCADDY"
run 1.0.0
assert_eq "host-proxy, placeholder ACME_EMAIL: deploys" 0 "$rc"
setup $'ACME_EMAIL=CHANGE_ME_you@example.com\nPOSTGRES_PASSWORD=CHANGE_ME_openssl_rand_hex_24' "$NOCADDY"
run 1.0.0
assert_eq "host-proxy, another placeholder: refused" 1 "$rc"
assert_has "host-proxy, another placeholder: listed" "$out" "POSTGRES_PASSWORD=..."
assert_lacks "host-proxy, another placeholder: ACME_EMAIL is not listed" "$out" "ACME_EMAIL=..."

# 9. image cleanup touches Steerpost images only: every prune carries a Steerpost label filter, never a bare prune
setup
run 1.0.0
assert_eq "deploy for the prune check: exit" 0 "$rc"
assert_has "prune: it does clean up" "$(calls)" ":: image prune"
assert_eq "prune: no call without a Steerpost label filter" 0 \
  "$(calls | grep ' :: image prune' | grep -vc 'label=org.opencontainers.image.title=socialos-' || true)"

# 10. after a manual rollback the operator is told the timer would undo it
setup
mkdir -p "$SB/app/.deploy"
echo 1.0.0 >"$SB/app/.deploy/current_tag"
echo 0.9.0 >"$SB/app/.deploy/previous_tag"
run --rollback
assert_eq "rollback: exit" 0 "$rc"
assert_has "rollback: autoupdate hint" "$out" "set AUTOUPDATE=false in .env now"
setup
run 1.0.0
assert_lacks "plain deploy: no rollback hint" "$out" "AUTOUPDATE=false"

finish
