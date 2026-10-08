#!/usr/bin/env bash
# Create ./.env for the production stack from .env.prod.example, with freshly generated secrets.
#
#   ./init-env.sh <domain> <acme-email>                 e.g. ./init-env.sh example.com ops@example.com
#   ./init-env.sh --host-proxy <domain> [<acme-email>]  behind the host's existing reverse proxy (README, host-proxy mode)
#
# Never overwrites an existing .env. Afterwards add LINKEDIN_* and TELEGRAM_BOT_TOKEN, or leave them empty.
# With --host-proxy, COMPOSE_FILE also includes docker-compose.host-proxy.yml, and <acme-email> is optional: only the
# bundled Caddy uses it, and that does not run in this mode (the host's Caddy handles certificates). Left out, it stays empty.
set -Eeuo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"

host_proxy=false
if [ "${1-}" = "--host-proxy" ]; then
  host_proxy=true
  shift
fi
if [ "$host_proxy" = true ]; then
  min_args=1
else
  min_args=2
fi
if [ $# -lt "$min_args" ] || [ $# -gt 2 ]; then
  echo "usage: $0 <domain> <acme-email> | $0 --host-proxy <domain> [<acme-email>]" >&2
  exit 2
fi
domain=$1
email=${2-}
[[ "$domain" =~ ^[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?$ ]] || {
  echo "invalid domain: $domain" >&2
  exit 2
}
if [ -n "$email" ] || [ "$host_proxy" = false ]; then
  [[ "$email" =~ ^[^@[:space:]|]+@[^@[:space:]|]+$ ]] || {
    echo "invalid email: $email" >&2
    exit 2
  }
fi
[ ! -e .env ] || {
  echo ".env already exists; refusing to overwrite it" >&2
  exit 1
}
command -v openssl >/dev/null 2>&1 || {
  echo "openssl is required" >&2
  exit 1
}

umask 077
cp .env.prod.example .env

# set_var KEY VALUE: replace the (uncommented) KEY=... line. VALUE must not contain "|" or newlines.
set_var() {
  local key=$1 value=$2
  grep -q "^${key}=" .env || {
    echo "internal error: ${key} is missing in .env.prod.example" >&2
    exit 1
  }
  sed -i "s|^${key}=.*|${key}=${value}|" .env
}

set_var DOMAIN "$domain"
set_var ACME_EMAIL "$email"
set_var POSTGRES_PASSWORD "$(openssl rand -hex 24)"
set_var REDIS_PASSWORD "$(openssl rand -hex 24)"
set_var ENCRYPTION_KEY "$(openssl rand -base64 32)"
set_var METRICS_TOKEN "$(openssl rand -hex 24)"
set_var S3_ACCESS_KEY "$(openssl rand -hex 8)"
set_var S3_SECRET_KEY "$(openssl rand -hex 24)"
set_var TELEGRAM_WEBHOOK_SECRET "$(openssl rand -hex 24)"
if [ "$host_proxy" = true ]; then set_var COMPOSE_FILE "docker-compose.prod.yml:docker-compose.host-proxy.yml"; fi
set_var MCP_GATEWAY_SECRET "$(openssl rand -hex 32)"

echo "wrote $(pwd)/.env (mode 600) with fresh secrets for ${domain}."
echo "Back it up somewhere safe: ENCRYPTION_KEY cannot be recovered, and without it stored social-account tokens are lost."
if [ "$host_proxy" = true ]; then
  echo "Host-proxy mode: next ./host-proxy/render-caddy.sh and ./host-proxy/install-caddy-import.sh, then ./deploy.sh <image-tag> (README, section 13)."
else
  echo "Next: set LINKEDIN_CLIENT_ID/SECRET and TELEGRAM_BOT_TOKEN in .env if you use them, then ./deploy.sh <image-tag>"
fi
