#!/usr/bin/env bash
# Print the body of one version's section of a Keep a Changelog file: everything under "## [VERSION]" up to the next
# "## [" heading or the link definitions at the bottom. The heading line itself is left out; blank lines at both ends are trimmed.
#
#   .github/scripts/changelog-section.sh 0.1.0 [CHANGELOG.md]
#
# VERSION is the tag without the "v". A pre-release (0.2.0-rc.1) uses its own section if there is one, otherwise the section
# of the release it leads up to (0.2.0). Exit status: 0 printed; 1 no such section, or it is empty; 2 bad arguments.
# Used by release.yml to write the notes of the GitHub Release (autoupdate.sh on the servers polls that release).
set -euo pipefail

version=${1:-}
file=${2:-CHANGELOG.md}
if ! [[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
  echo "changelog-section: usage: $0 <X.Y.Z[-pre]> [file] (got '$version')" >&2
  exit 2
fi
[ -f "$file" ] || {
  echo "changelog-section: $file not found" >&2
  exit 2
}

# section VERSION: the body, trimmed. The heading must be "## [VERSION]" exactly, followed by nothing or by a space (a date).
section() {
  awk -v want="$1" '
    BEGIN { prefix = "## [" want "]"; n = length(prefix) }
    /^## \[/ {
      if (on) exit
      if (substr($0, 1, n) == prefix) {
        rest = substr($0, n + 1)
        if (rest == "" || rest ~ /^[ \t]/) { on = 1; next }
      }
    }
    on && /^\[[^]]+\]:[ \t]/ { exit }
    on { lines[++count] = $0 }
    END {
      first = 1
      while (first <= count && lines[first] ~ /^[ \t]*$/) first++
      last = count
      while (last >= first && lines[last] ~ /^[ \t]*$/) last--
      for (i = first; i <= last; i++) print lines[i]
    }
  ' "$file"
}

body=$(section "$version")
if [ -z "$body" ] && [[ "$version" == *-* ]]; then body=$(section "${version%%-*}"); fi
if [ -z "$body" ]; then
  echo "changelog-section: $file has no non-empty section '## [$version]'. Add it (and move the entries out of [Unreleased]) before tagging v$version." >&2
  exit 1
fi
printf '%s\n' "$body"
