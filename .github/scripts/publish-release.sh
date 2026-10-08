#!/usr/bin/env bash
# Create the GitHub Release for a tag, or update the notes of the one that already exists (so a re-run is harmless).
#
#   .github/scripts/publish-release.sh vX.Y.Z notes.md
#
# A tag with a pre-release suffix (v0.2.0-rc.1) is created with --prerelease, so it can never become "latest": that is the
# release autoupdate.sh on the servers follows. --verify-tag makes `gh` refuse to invent a tag that is not on the remote.
# Needs the GitHub CLI with GH_TOKEN (contents: write) and GH_REPO or a git remote.
set -euo pipefail

tag=${1:-}
notes=${2:-}
if ! [[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || [ ! -s "$notes" ]; then
  echo "publish-release: usage: $0 vX.Y.Z[-pre] <non-empty notes file> (got '$tag' '$notes')" >&2
  exit 2
fi

flags=()
if [[ "$tag" == *-* ]]; then flags+=(--prerelease); fi

if gh release view "$tag" >/dev/null 2>&1; then
  echo "publish-release: release $tag exists, updating its notes"
  gh release edit "$tag" --title "$tag" --notes-file "$notes" "${flags[@]}"
else
  echo "publish-release: creating release $tag"
  gh release create "$tag" --verify-tag --title "$tag" --notes-file "$notes" "${flags[@]}"
fi
