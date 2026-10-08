#!/usr/bin/env bash
# Render the Caddy sites for host-proxy mode from .env, with concrete hostnames and ports.
#
#   ./host-proxy/render-caddy.sh                  # .env -> /opt/socialos/caddy/socialos.caddy
#   ./host-proxy/render-caddy.sh --out -          # print instead of writing
#   ./host-proxy/render-caddy.sh --env FILE --out FILE
#
# Reads from .env: DOMAIN (required), S3_SITE (default s3.$DOMAIN; a value starting with ":" turns the s3 site off),
# COMPOSE_PROFILES (the s3 site is also left out when "minio" is not in it and S3_SITE is unset), and the host ports
# FRONTEND_HOST_PORT=13000, BACKEND_HOST_PORT=18080, MCP_HOST_PORT=13333, MINIO_HOST_PORT=19000.
# The template is socialos.caddy.tmpl next to this script. Nothing is changed on the host Caddy: the result is only
# imported once install-caddy-import.sh has added the import line.
set -Eeuo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
root_dir=$(dirname "$script_dir") # /opt/socialos
env_file="$root_dir/.env"
out_file="$root_dir/caddy/socialos.caddy"
template="$script_dir/socialos.caddy.tmpl"

die() {
  echo "render-caddy: $*" >&2
  exit 1
}
usage() { sed -n '2,12p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; }

while [ $# -gt 0 ]; do
  case "$1" in
    --env)
      [ $# -ge 2 ] || die "--env needs a file"
      env_file=$2
      shift
      ;;
    --out)
      [ $# -ge 2 ] || die "--out needs a file (or - for stdout)"
      out_file=$2
      shift
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *) die "unknown argument: $1" ;;
  esac
  shift
done

[ -f "$env_file" ] || die "$env_file not found (create it with ./init-env.sh)"
[ -f "$template" ] || die "$template not found"

# env_get KEY: last assignment of KEY in the env file, without surrounding quotes.
env_get() {
  local line
  line=$(grep -E "^[[:space:]]*$1=" "$env_file" | tail -n 1 || true)
  line=${line#*=}
  line=${line%\"}
  line=${line#\"}
  line=${line%\'}
  line=${line#\'}
  printf '%s' "$line"
}

host_re='^[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?$'

domain=$(env_get DOMAIN)
[ -n "$domain" ] || die "DOMAIN is not set in $env_file"
[[ "$domain" =~ $host_re ]] || die "DOMAIN is not a valid hostname: $domain"

port_of() { # port_of KEY DEFAULT
  local value
  value=$(env_get "$1")
  value=${value:-$2}
  if ! [[ "$value" =~ ^[0-9]{1,5}$ ]] || [ "$value" -lt 1 ] || [ "$value" -gt 65535 ]; then
    die "$1 is not a port number: $value"
  fi
  printf '%s' "$value"
}
frontend_port=$(port_of FRONTEND_HOST_PORT 13000)
backend_port=$(port_of BACKEND_HOST_PORT 18080)
mcp_port=$(port_of MCP_HOST_PORT 13333)
minio_port=$(port_of MINIO_HOST_PORT 19000)

# The s3 site is only wanted with the bundled MinIO. With external S3/R2 the operator sets S3_SITE=:8099 (see
# .env.prod.example). On a host proxy such a site would open a public port 8099, so it is left out entirely.
s3_site=$(env_get S3_SITE)
s3_enabled=1
if [ -z "$s3_site" ]; then
  s3_site="s3.$domain"
  case ",$(env_get COMPOSE_PROFILES)," in
    *,minio,*) ;;
    *) s3_enabled=0 ;;
  esac
elif [[ "$s3_site" == :* ]]; then
  s3_enabled=0
else
  [[ "$s3_site" =~ $host_re ]] || die "S3_SITE is neither a hostname nor :<port> (to switch it off): $s3_site"
fi

render() {
  local line skip=0
  while IFS= read -r line || [ -n "$line" ]; do
    case $line in
      '# @@S3_BEGIN@@')
        [ "$s3_enabled" = 1 ] || skip=1
        continue
        ;;
      '# @@S3_END@@')
        skip=0
        continue
        ;;
    esac
    [ "$skip" = 0 ] || continue
    line=${line//@DOMAIN@/$domain}
    line=${line//@S3_SITE@/$s3_site}
    line=${line//@FRONTEND_PORT@/$frontend_port}
    line=${line//@BACKEND_PORT@/$backend_port}
    line=${line//@MCP_PORT@/$mcp_port}
    line=${line//@MINIO_PORT@/$minio_port}
    printf '%s\n' "$line"
  done <"$template"
}

if [ "$out_file" = "-" ]; then
  render
  exit 0
fi

mkdir -p "$(dirname "$out_file")"
tmp=$(mktemp "$out_file.XXXXXX")
trap 'rm -f "$tmp"' EXIT
render >"$tmp"
chmod 644 "$tmp" # the host's caddy user must be able to read it; it holds no secrets
mv -f "$tmp" "$out_file"
trap - EXIT
echo "render-caddy: wrote $out_file ($domain; frontend :$frontend_port, backend :$backend_port, mcp :$mcp_port$([ "$s3_enabled" = 1 ] && echo ", s3 $s3_site :$minio_port" || echo ", s3 site off"))"
