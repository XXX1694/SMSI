#!/usr/bin/env bash
# Tiny assertion helpers for the deploy script tests. Source it from a *.test.sh file. No dependencies.
# shellcheck disable=SC2034 # the variables are read by the test files that source this one
T_PASS=0
T_FAIL=0
REPO_DEPLOY=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
T_ROOT=$(mktemp -d)
trap 'rm -rf "$T_ROOT"' EXIT

ok() { T_PASS=$((T_PASS + 1)); }
bad() {
  T_FAIL=$((T_FAIL + 1))
  printf 'FAIL  %s\n' "$1" >&2
  if [ -n "${2-}" ]; then printf '      %s\n' "$2" >&2; fi
}
assert_eq() { if [ "$2" = "$3" ]; then ok; else bad "$1" "expected '$2', got '$3'"; fi; }
assert_has() { # name haystack needle
  case $2 in *"$3"*) ok ;; *) bad "$1" "missing '$3' in: $(printf '%s' "$2" | head -c 500)" ;; esac
}
assert_lacks() { # name haystack needle
  case $2 in *"$3"*) bad "$1" "unexpected '$3' in: $(printf '%s' "$2" | head -c 500)" ;; *) ok ;; esac
}
assert_file() { if [ -e "$2" ]; then ok; else bad "$1" "missing file $2"; fi; }
assert_no_file() { if [ ! -e "$2" ]; then ok; else bad "$1" "unexpected file $2"; fi; }

# new_sb: a fresh empty directory in $SB for one test case.
new_sb() {
  SB=$(mktemp -d "$T_ROOT/case.XXXXXX")
  mkdir -p "$SB/bin"
}
finish() {
  printf '%s: %s passed, %s failed\n' "$(basename "$0")" "$T_PASS" "$T_FAIL"
  [ "$T_FAIL" -eq 0 ]
}
