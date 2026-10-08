#!/usr/bin/env bash
# .github/scripts/changelog-section.sh (release notes from CHANGELOG.md) and .github/scripts/publish-release.sh (create or
# update the GitHub Release; gh is stubbed), plus a guard that the real CHANGELOG.md is in the format release.yml needs.
# shellcheck source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

SCRIPTS=$REPO_DEPLOY/../.github/scripts
CHANGELOG=$REPO_DEPLOY/../CHANGELOG.md

section() { # section VERSION [FILE]: sets $out (stdout only) and $rc, and $err
  rc=0
  err=$SB/err
  out=$(bash "$SCRIPTS/changelog-section.sh" "$@" 2>"$err") || rc=$?
  err=$(cat "$err")
}

new_sb
cat >"$SB/CHANGELOG.md" <<'MD'
# Changelog

Intro text.

## [Unreleased]

- not released yet

## [0.10.0] - 2026-11-01

### Added

- ten

## [0.2.0-rc.1] - 2026-10-20

- the candidate

## [0.2.0]

### Fixed

- two

## [0.1.0] - 2026-10-09

First release.

### Added

- one
- **bold** and `code` and [a link](https://example.org)

[Unreleased]: https://example.org/compare/v0.10.0...HEAD
[0.1.0]: https://example.org/releases/tag/v0.1.0
MD
F=$SB/CHANGELOG.md

section 0.1.0 "$F"
assert_eq "0.1.0: exit" 0 "$rc"
# shellcheck disable=SC2016 # the backticks are literal Markdown, not a command substitution
assert_eq "0.1.0: body without heading, trimmed, up to the link definitions" "$(printf 'First release.\n\n### Added\n\n- one\n- **bold** and `code` and [a link](https://example.org)')" "$out"
section 0.10.0 "$F"
assert_eq "0.10.0 is not confused with 0.1.0: body" "$(printf '### Added\n\n- ten')" "$out"
section 0.2.0 "$F"
assert_eq "0.2.0: body stops at the next heading" "$(printf '### Fixed\n\n- two')" "$out"
section 0.2.0-rc.1 "$F"
assert_eq "rc with its own section uses it" "- the candidate" "$out"
section 0.2.0-rc.2 "$F"
assert_eq "rc without a section falls back to the release it leads up to" "$(printf '### Fixed\n\n- two')" "$out"
section 0.3.0-rc.1 "$F"
assert_eq "rc without any section: exit" 1 "$rc"

section 0.3.0 "$F"
assert_eq "missing section: exit" 1 "$rc"
assert_eq "missing section: nothing on stdout" "" "$out"
assert_has "missing section: says which" "$err" "## [0.3.0]"
section 1.1.0 "$F"
assert_eq "[Unreleased] is never a release's notes: exit" 1 "$rc"
section 0x1x0 "$F"
assert_eq "malformed version: exit" 2 "$rc"
section "0.1.0; rm -rf /" "$F"
assert_eq "injection attempt: exit" 2 "$rc"
section 0.1.0 "$SB/nope.md"
assert_eq "missing file: exit" 2 "$rc"

printf '## [0.4.0] - 2026-01-01\n\n\n## [0.3.9]\n\n- x\n' >"$SB/empty.md"
section 0.4.0 "$SB/empty.md"
assert_eq "empty section: exit" 1 "$rc"
printf '## [1x2x3] - d\n\n- dots are literal\n' >"$SB/dots.md"
section 1.2.3 "$SB/dots.md"
assert_eq "dots are literal, not wildcards: exit" 1 "$rc"
printf '## [0.5.0]-ish\n\n- no\n## [0.5.0] - d\n\n- yes\n' >"$SB/ish.md"
section 0.5.0 "$SB/ish.md"
assert_eq "only the exact heading matches" "- yes" "$out"

# the real file: has [Unreleased], and every released section is non-empty and linked at the bottom
assert_has "CHANGELOG.md has an Unreleased section" "$(cat "$CHANGELOG")" "## [Unreleased]"
versions=$(grep -oE '^## \[[0-9]+\.[0-9]+\.[0-9]+[^]]*\]' "$CHANGELOG" | sed 's/^## \[//; s/\]$//')
assert_has "CHANGELOG.md has a first release" "$versions" "0.1.0"
for v in $versions; do
  section "$v" "$CHANGELOG"
  assert_eq "CHANGELOG.md [$v]: section is non-empty" 0 "$rc"
  assert_has "CHANGELOG.md [$v]: version is linked at the bottom" "$(cat "$CHANGELOG")" "[$v]: https://github.com/"
done
assert_has "CHANGELOG.md links [Unreleased]" "$(cat "$CHANGELOG")" "[Unreleased]: https://github.com/"

# ---- publish-release.sh with a stubbed gh ------------------------------------------------------------------------------
setup_gh() {
  new_sb
  echo "notes" >"$SB/notes.md"
  cat >"$SB/bin/gh" <<STUB
#!/usr/bin/env bash
echo "\$*" >>"$SB/gh.calls"
if [ "\$1 \$2" = "release view" ]; then [ -e "$SB/exists" ]; exit; fi
exit 0
STUB
  chmod +x "$SB/bin/gh"
}
publish() { # publish ARGS: sets $out, $rc
  rc=0
  out=$(PATH="$SB/bin:$PATH" bash "$SCRIPTS/publish-release.sh" "$@" 2>&1) || rc=$?
}
gh_calls() { cat "$SB/gh.calls" 2>/dev/null || true; }

setup_gh
publish v0.1.0 "$SB/notes.md"
assert_eq "new release: exit" 0 "$rc"
assert_eq "new release: create with --verify-tag, title and notes, not a pre-release" \
  "$(printf 'release view v0.1.0\nrelease create v0.1.0 --verify-tag --title v0.1.0 --notes-file %s/notes.md' "$SB")" "$(gh_calls)"

setup_gh
publish v0.2.0-rc.1 "$SB/notes.md"
assert_has "rc: created as a pre-release (never latest)" "$(gh_calls)" "release create v0.2.0-rc.1 --verify-tag --title v0.2.0-rc.1 --notes-file $SB/notes.md --prerelease"

setup_gh
touch "$SB/exists"
publish v0.1.0 "$SB/notes.md"
assert_eq "existing release: exit" 0 "$rc"
assert_eq "existing release: only its notes are updated" \
  "$(printf 'release view v0.1.0\nrelease edit v0.1.0 --title v0.1.0 --notes-file %s/notes.md' "$SB")" "$(gh_calls)"
assert_has "existing release: says so" "$out" "exists, updating"
publish v0.1.0 "$SB/notes.md"
assert_eq "running it again is harmless" 0 "$rc"
setup_gh
touch "$SB/exists"
publish v0.2.0-rc.1 "$SB/notes.md"
assert_has "existing rc stays a pre-release" "$(gh_calls)" "release edit v0.2.0-rc.1 --title v0.2.0-rc.1 --notes-file $SB/notes.md --prerelease"

for badtag in 0.1.0 v1.2 latest "v1.2.3; echo hi" ""; do
  setup_gh
  publish "$badtag" "$SB/notes.md"
  assert_eq "tag '$badtag': refused" 2 "$rc"
  assert_eq "tag '$badtag': gh not called" "" "$(gh_calls)"
done
setup_gh
: >"$SB/empty-notes.md"
publish v0.1.0 "$SB/empty-notes.md"
assert_eq "empty notes file: refused" 2 "$rc"
publish v0.1.0 "$SB/missing.md"
assert_eq "missing notes file: refused" 2 "$rc"
assert_eq "refused runs never call gh" "" "$(gh_calls)"

finish
