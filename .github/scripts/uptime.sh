#!/usr/bin/env bash
# Uptime probe. Checks each URL, keeps one GitHub issue per outage (label "incident").
# Env: UPTIME_URLS (space separated) or API_PUBLIC_URL [+ MCP_PUBLIC_URL] to derive targets;
#      GH_TOKEN / GH_REPO for gh; UPTIME_ATTEMPTS (3), UPTIME_BACKOFF seconds (5), UPTIME_TIMEOUT (10),
#      UPTIME_CERT_WARN_DAYS (14).
set -uo pipefail

attempts="${UPTIME_ATTEMPTS:-3}"
backoff="${UPTIME_BACKOFF:-5}"
timeout_s="${UPTIME_TIMEOUT:-10}"
warn_days="${UPTIME_CERT_WARN_DAYS:-14}"

derive_urls() {
  local api="${API_PUBLIC_URL:-}" mcp="${MCP_PUBLIC_URL:-}" out=() scheme
  api="${api%/}"
  if [ -n "$api" ]; then
    out+=("$api/ready")
    case "$api" in
      *://api.*) scheme="${api%%://*}"; out+=("${scheme}://app.${api#*://api.}/login") ;;
    esac
  fi
  if [ -n "$mcp" ]; then
    mcp="${mcp%/}"
    out+=("${mcp%/mcp}/health")
  fi
  echo "${out[*]:-}"
}

probe() { # url -> 0 when 2xx/3xx within attempts
  local url="$1" i code
  for ((i = 1; i <= attempts; i++)); do
    code=$(curl -s -o /dev/null -m "$timeout_s" -w '%{http_code}' "$url" 2>/dev/null || true)
    case "$code" in 2* | 3*) return 0 ;; esac
    echo "attempt $i/$attempts: HTTP ${code:-000}" >&2
    if [ "$i" -lt "$attempts" ]; then sleep "$((backoff * i))"; fi
  done
  return 1
}

check_cert() { # url: warn when the TLS certificate expires soon
  local url="$1" hostport host port end
  case "$url" in https://*) ;; *) return 0 ;; esac
  hostport="${url#https://}"
  hostport="${hostport%%/*}"
  host="${hostport%%:*}"
  port=443
  case "$hostport" in *:*) port="${hostport##*:}" ;; esac
  end=$(echo | openssl s_client -servername "$host" -connect "$host:$port" 2>/dev/null |
    openssl x509 -noout -enddate 2>/dev/null) || true
  [ -n "$end" ] || { echo "::warning::Could not read the TLS certificate of $host"; return 0; }
  if ! echo | openssl s_client -servername "$host" -connect "$host:$port" 2>/dev/null |
    openssl x509 -noout -checkend "$((warn_days * 86400))" >/dev/null 2>&1; then
    echo "::warning::TLS certificate of $host expires in under $warn_days days (${end#notAfter=})"
  fi
}

to_epoch() {
  date -u -d "$1" +%s 2>/dev/null || date -u -j -f '%Y-%m-%dT%H:%M:%SZ' "$1" +%s 2>/dev/null || echo 0
}

human() { # seconds -> "1h 5m"
  local s="$1"
  if [ "$s" -ge 3600 ]; then echo "$((s / 3600))h $((s % 3600 / 60))m"
  elif [ "$s" -ge 60 ]; then echo "$((s / 60))m"
  else echo "${s}s"; fi
}

open_issue() { # title -> "number createdAt" or empty
  gh issue list --state open --label incident --search "in:title \"$1\"" \
    --json number,title,createdAt \
    --jq "[.[] | select(.title == \"$1\")] | .[0] | select(. != null) | \"\(.number) \(.createdAt)\""
}

urls="${UPTIME_URLS:-}"
[ -n "$urls" ] || urls="$(derive_urls)"
if [ -z "$urls" ]; then
  echo "::notice::No uptime targets: set the UPTIME_URLS or API_PUBLIC_URL repository variable."
  exit 0
fi

failed=0
for url in $urls; do
  title="Uptime: $url is down"
  existing="$(open_issue "$title")"
  if probe "$url"; then
    echo "OK   $url"
    check_cert "$url"
    if [ -n "$existing" ]; then
      num="${existing%% *}"
      since="$(to_epoch "${existing#* }")"
      dur="$(human "$(($(date -u +%s) - since))")"
      gh issue comment "$num" --body "$url recovered after $dur." >/dev/null
      gh issue close "$num" >/dev/null
      echo "Closed issue #$num"
    fi
  else
    failed=1
    echo "DOWN $url"
    body="$url did not return 2xx/3xx in $attempts attempts (timeout ${timeout_s}s) at $(date -u +%FT%TZ)."
    if [ -n "$existing" ]; then
      gh issue comment "${existing%% *}" --body "Still down. $body" >/dev/null
    else
      gh label create incident --color D93F0B --description "Service incident" >/dev/null 2>&1 || true
      gh issue create --title "$title" --label incident --body "$body" >/dev/null
    fi
  fi
done

# Which build answered, for the run log only: a backend without /version (0.4.0 and older) must not fail the probe.
# Only the three parsed fields are printed, never the raw body, and each is cut to version-like characters (no newline
# can start a line): whatever answers that URL could otherwise inject workflow commands (::stop-commands::) into the log.
if [ -n "${API_PUBLIC_URL:-}" ]; then
  version="$(curl -fsS -m "$timeout_s" --max-filesize 2048 "${API_PUBLIC_URL%/}/version" 2>/dev/null |
    jq -r '[.version, .commit, .built_at] | map(tostring | gsub("[^0-9A-Za-z._:+-]"; "")) | join(" ")' 2>/dev/null ||
    true)"
  echo "Version: ${version:-unknown}"
fi
exit "$failed"
