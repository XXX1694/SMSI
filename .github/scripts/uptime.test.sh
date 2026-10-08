#!/usr/bin/env bash
# Tests for uptime.sh: stubbed gh, real curl against a local HTTP server.
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
work="$(mktemp -d)"
trap 'kill "${srv:-}" 2>/dev/null || true; wait "${srv:-}" 2>/dev/null || true; rm -rf "$work"' EXIT
mkdir -p "$work/bin" "$work/www"
echo ok >"$work/www/up"

# gh stub: state in $work (issues file holds "num|title|createdAt"), calls logged to calls.log.
cat >"$work/bin/gh" <<'STUB'
#!/usr/bin/env bash
echo "gh $*" >>"$STUB_DIR/calls.log"
case "$1 $2" in
  "issue list") [ -s "$STUB_DIR/issue" ] && { IFS='|' read -r n t c <"$STUB_DIR/issue"; echo "$n $c"; } || true ;;
  "issue create") echo "42|$(printf '%s\n' "$@" | sed -n '/^--title$/{n;p;}')|2020-01-01T00:00:00Z" >"$STUB_DIR/issue" ;;
  "issue close") : >"$STUB_DIR/issue" ;;
esac
STUB
chmod +x "$work/bin/gh"

python3 -u -m http.server 0 --bind 127.0.0.1 --directory "$work/www" >"$work/srv.log" 2>&1 &
srv=$!
for _ in $(seq 50); do
  port="$(grep -o 'port [0-9]*' "$work/srv.log" | head -1 | cut -d' ' -f2 || true)"
  [ -n "$port" ] && break
  sleep 0.1
done
[ -n "${port:-}" ] || { echo "server did not start"; exit 1; }

export STUB_DIR="$work" PATH="$work/bin:$PATH" UPTIME_BACKOFF=0 UPTIME_ATTEMPTS=2 UPTIME_TIMEOUT=2
fails=0
count() { grep -c "^gh $1" "$work/calls.log" 2>/dev/null || true; }
expect() { # desc actual expected
  if [ "$2" = "$3" ]; then echo "ok   $1"; else echo "FAIL $1: got '$2', want '$3'"; fails=1; fi
}
run() { UPTIME_URLS="$1" bash "$here/uptime.sh" >/dev/null 2>&1 && echo 0 || echo $?; }

: >"$work/calls.log"
: >"$work/issue"
expect "healthy: exit 0" "$(run "http://127.0.0.1:$port/up")" 0
expect "healthy: no issue" "$(count 'issue create')" 0

expect "down: exit 1" "$(run "http://127.0.0.1:$port/missing")" 1
expect "down: issue opened once" "$(count 'issue create')" 1
grep -q 'issue create.*--label incident' "$work/calls.log" && r=yes || r=no
expect "down: labelled incident" "$r" yes

run "http://127.0.0.1:$port/missing" >/dev/null
expect "second failure: still one issue" "$(count 'issue create')" 1
expect "second failure: commented" "$(count 'issue comment')" 1

# Recovery: the issue title must match the probed URL, so reopen it for the "up" URL.
echo "42|Uptime: http://127.0.0.1:$port/up is down|2020-01-01T00:00:00Z" >"$work/issue"
: >"$work/calls.log"
expect "recovery: exit 0" "$(run "http://127.0.0.1:$port/up")" 0
expect "recovery: commented" "$(count 'issue comment')" 1
grep -q 'recovered after' "$work/calls.log" && r=yes || r=no
expect "recovery: says recovered after" "$r" yes
expect "recovery: closed" "$(count 'issue close')" 1

# No targets: skip with a notice.
out="$(env -u UPTIME_URLS -u API_PUBLIC_URL -u MCP_PUBLIC_URL bash "$here/uptime.sh")"
case "$out" in *notice*) r=yes ;; *) r=no ;; esac
expect "no targets: notice" "$r" yes

# Derived targets.
sed -n '/^derive_urls/,/^}/p' "$here/uptime.sh" >"$work/derive.sh"
out="$(API_PUBLIC_URL=https://api.example.com MCP_PUBLIC_URL=https://mcp.example.com/mcp \
  bash -c "source '$work/derive.sh'; derive_urls")"
expect "derived urls" "$out" "https://api.example.com/ready https://app.example.com/login https://mcp.example.com/health"

exit "$fails"
