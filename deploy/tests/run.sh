#!/usr/bin/env bash
# Runs every deploy/tests/*.test.sh (the shell scripts in deploy/ against stubbed docker, curl, caddy and systemctl;
# no network, no Docker daemon). Needs Linux because deploy.sh and autoupdate.sh need flock. On macOS:
#   docker run --rm -v "$PWD:/repo" -w /repo ubuntu:24.04 bash deploy/tests/run.sh
set -u
cd "$(dirname "${BASH_SOURCE[0]}")" || exit 2
command -v flock >/dev/null 2>&1 || {
  echo "run.sh: flock not found (these tests need Linux, see the comment at the top)" >&2
  exit 2
}
rc=0
for t in ./*.test.sh; do
  bash "$t" || rc=1
done
exit "$rc"
