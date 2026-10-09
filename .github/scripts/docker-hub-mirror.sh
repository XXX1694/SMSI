#!/usr/bin/env bash
# Points the runner's Docker daemon at Google's Docker Hub cache (mirror.gcr.io) before anything pulls an image.
# Why: GitHub-hosted runners share egress IPs, and Docker Hub's anonymous pull limit answered 429 for hours on
# 2026-10-09, failing every E2E and actionlint run. The daemon still falls back to Docker Hub for an image the mirror
# lacks. Buildx builds get the same mirror through `buildkitd-config-inline` on setup-buildx-action.
# Usage: sudo .github/scripts/docker-hub-mirror.sh   (CI only; it restarts dockerd)
set -Eeuo pipefail

config=/etc/docker/daemon.json
current='{}'
if [[ -s "$config" ]]; then current=$(cat "$config"); fi
jq '."registry-mirrors" = ((."registry-mirrors" // []) + ["https://mirror.gcr.io"] | unique)' <<<"$current" >"$config.new"
mv "$config.new" "$config"
systemctl restart docker
docker info --format '{{json .RegistryConfig.Mirrors}}'
