#!/usr/bin/env bash
# Static checks of the production compose files and Caddy config, run by the repo-lint job in ci.yml.
# Usage: .github/scripts/repo-lint.sh compose | caddy | host-proxy   (from the repository root)
# Needs docker, jq and the example environment in deploy/. Exit codes: 0 ok, 1 a check failed, 2 bad usage.
set -Eeuo pipefail

work_root="${RUNNER_TEMP:-${TMPDIR:-/tmp}}"

check_compose() {
  standalone=(--env-file deploy/.env.prod.example -f deploy/docker-compose.prod.yml)
  host_proxy=("${standalone[@]}" -f deploy/docker-compose.host-proxy.yml)
  # check MODE JQ_FILTER MESSAGE: the resolved configuration of MODE must satisfy the jq filter.
  check() {
    local -n compose_args=$1
    docker compose "${compose_args[@]}" config --format json | jq -e "$2" >/dev/null ||
      { echo "::error title=compose config ($1)::$3"; exit 1; }
  }
  docker compose "${standalone[@]}" config -q
  docker compose "${host_proxy[@]}" config -q

  check standalone '[.services[] | select(.mem_limit == null)] | length == 0' 'a service has no mem_limit'
  check host_proxy '[.services[] | select(.mem_limit == null)] | length == 0' 'a service has no mem_limit'
  # Standalone is unchanged: caddy owns 80/443 and nothing else publishes a port.
  check standalone '[.services | to_entries[] | select(.value.ports != null) | .key] == ["caddy"]' 'only caddy may publish ports'
  # Host-proxy: no bundled caddy, and everything that is published is bound to 127.0.0.1.
  check host_proxy '.services | has("caddy") | not' 'the bundled caddy is still part of the stack'
  check host_proxy '[.services[].ports[]?.host_ip] | (length > 0) and all(. == "127.0.0.1")' 'a published port is not bound to 127.0.0.1'
  # Host-proxy: under host-wide memory pressure the kernel must pick Steerpost before the host's Caddy or other service,
  # and postgres gets a small /dev/shm (64 MiB, not smaller than its shared_buffers default of 64MB).
  check host_proxy '[.services[] | select(.oom_score_adj != 500)] | length == 0' 'a service lacks oom_score_adj 500'
  check host_proxy '.services.postgres.shm_size | tonumber == 67108864' 'postgres shm_size is not 64mb'
  # Host-proxy: the whole stack runs in socialos.slice (one kernel-enforced budget), no container may swap, and each
  # has a process cap. Standalone needs no slice on the host.
  check host_proxy '[.services[] | select(.cgroup_parent != "socialos.slice")] | length == 0' 'a service is not in socialos.slice'
  check host_proxy '[.services[] | select(.memswap_limit != .mem_limit or .pids_limit == null)] | length == 0' 'a service may swap or has no pids_limit'
  check standalone '[.services[] | select(.cgroup_parent != null)] | length == 0' 'the standalone stack must not depend on socialos.slice'
  # ACME_EMAIL is only for the bundled Caddy: host-proxy mode must resolve with it empty (init-env.sh --host-proxy writes
  # it empty). Standalone resolves too, and deploy.sh refuses an empty value there.
  sed 's|^ACME_EMAIL=.*|ACME_EMAIL=|' deploy/.env.prod.example >"$work_root/env-no-acme"
  docker compose --env-file "$work_root/env-no-acme" -f deploy/docker-compose.prod.yml -f deploy/docker-compose.host-proxy.yml config -q
}

check_caddy() {
  docker run --rm \
    -v "$PWD/deploy/Caddyfile:/etc/caddy/Caddyfile:ro" \
    -e DOMAIN=example.com -e ACME_EMAIL=ops@example.com -e S3_SITE=s3.example.com \
    caddy:2-alpine caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile
}

check_host_proxy() {
  work="$work_root/host-proxy"
  mkdir -p "$work/caddy"
  sed 's|^DOMAIN=.*|DOMAIN=example.com|' deploy/.env.prod.example >"$work/env"
  deploy/host-proxy/render-caddy.sh --env "$work/env" --out "$work/caddy/socialos.caddy"
  cat >"$work/Caddyfile" <<'EOF'
{
	email ops@example.com
}
(common) {
	header X-Host "1"
}
other.example.org {
	import common
	respond "ok"
}
import /opt/socialos/caddy/*.caddy
EOF
  host_caddy=(docker run --rm -v "$work/Caddyfile:/etc/caddy/Caddyfile:ro" -v "$work/caddy:/opt/socialos/caddy:ro" caddy:2-alpine caddy)
  "${host_caddy[@]}" validate --config /etc/caddy/Caddyfile --adapter caddyfile

  # Routes without the upstream addresses (the host's own site is left out on the host side).
  "${host_caddy[@]}" adapt --config /etc/caddy/Caddyfile --adapter caddyfile 2>/dev/null |
    jq -S '[.apps.http.servers[].routes[] | select(.match[0].host[0] | startswith("other.") | not)] | map(del(.. | .upstreams?))' >"$work/host.json"
  docker run --rm -v "$PWD/deploy/Caddyfile:/etc/caddy/Caddyfile:ro" \
    -e DOMAIN=example.com -e ACME_EMAIL=ops@example.com -e S3_SITE=s3.example.com \
    caddy:2-alpine caddy adapt --config /etc/caddy/Caddyfile --adapter caddyfile 2>/dev/null |
    jq -S '[.apps.http.servers[].routes[]] | map(del(.. | .upstreams?))' >"$work/standalone.json"
  diff -u "$work/standalone.json" "$work/host.json"
}

case "${1:-}" in
  compose) check_compose ;;
  caddy) check_caddy ;;
  host-proxy) check_host_proxy ;;
  *) echo "usage: $0 compose|caddy|host-proxy" >&2; exit 2 ;;
esac
