#!/usr/bin/env bash
# restore-test.sh against a stubbed `docker`: PASS and FAIL reports, the newest dump is chosen, --network none, and the
# throwaway container is removed on every path.
# shellcheck source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

# setup [LIVE_TABLES] [LIVE_GOOSE]: sandbox with restore-test.sh and two dumps. The stub answers:
#   run -d                          -> starts the "container"        logs -> the readiness line twice (or never: $SB/never.ready)
#   exec ... createdb|pg_restore    -> fails when $SB/restore.fail exists (pg_restore consumes stdin)
#   exec ... psql                   -> restored copy: 12 tables / goose 34
#   compose ... exec -T postgres    -> live database: $LIVE_TABLES tables / goose $LIVE_GOOSE
setup() {
  new_sb
  mkdir -p "$SB/app" "$SB/bk"
  cp "$REPO_DEPLOY/restore-test.sh" "$SB/app/"
  echo older >"$SB/bk/socialos-db-20260101T000000Z.dump"
  echo newest >"$SB/bk/socialos-db-20260301T000000Z.dump"
  echo "${1-12}" >"$SB/live.tables"
  echo "${2-34}" >"$SB/live.goose"
  cat >"$SB/bin/docker" <<STUB
#!/usr/bin/env bash
echo "\$*" >>"$SB/calls"
case "\$1" in
  logs)
    [ -e "$SB/never.ready" ] || printf 'database system is ready to accept connections\ndatabase system is ready to accept connections\n'
    ;;
  exec)
    case "\$*" in
      *pg_restore*) cat >"$SB/restored.stdin"; [ ! -e "$SB/restore.fail" ] || exit 1 ;;
      *createdb*) ;;
      *"information_schema"*) echo 12 ;;
      *goose_db_version*) echo 34 ;;
    esac
    ;;
  compose)
    case "\$*" in
      *information_schema*) cat "$SB/live.tables" ;;
      *goose_db_version*) cat "$SB/live.goose" ;;
    esac
    ;;
esac
exit 0
STUB
  # the live queries pass their SQL as the last argument, so the stub above sees it in "$*"
  chmod +x "$SB/bin/docker"
}
run() { # run [ARGS]: sets $out and $rc
  rc=0
  out=$(cd "$SB/app" && PATH="$SB/bin:$PATH" BACKUP_DIR="$SB/bk" bash ./restore-test.sh "$@" 2>&1) || rc=$?
}
calls() { cat "$SB/calls" 2>/dev/null || true; }

# 1. PASS: the newest dump is restored through stdin into a container without network, which is removed afterwards
setup
run
assert_eq "pass: exit" 0 "$rc"
assert_has "pass: report" "$out" "restore test: PASS"
assert_has "pass: tables in the report" "$out" "tables  restored=12 live=12"
assert_has "pass: goose in the report" "$out" "goose   restored=34 live=34"
assert_has "pass: newest dump named" "$out" "socialos-db-20260301T000000Z.dump"
assert_eq "pass: newest dump restored" newest "$(cat "$SB/restored.stdin")"
assert_has "pass: no network" "$(calls)" "--network none"
assert_has "pass: postgres 16 alpine" "$(calls)" "postgres:16-alpine"
assert_has "pass: container removed" "$(calls)" "rm -f socialos-restore-test-"

# 2. an explicit file wins over the newest one
setup
run "$SB/bk/socialos-db-20260101T000000Z.dump"
assert_eq "explicit file: exit" 0 "$rc"
assert_eq "explicit file: restored" older "$(cat "$SB/restored.stdin")"

# 3. FAIL cases, each removes the container
setup 13 34
run
assert_eq "table mismatch: exit" 1 "$rc"
assert_has "table mismatch: report" "$out" "FAIL: table count differs"
assert_has "table mismatch: container removed" "$(calls)" "rm -f socialos-restore-test-"
setup 12 35
run
assert_eq "goose mismatch: exit" 1 "$rc"
assert_has "goose mismatch: report" "$out" "FAIL: goose version differs"
setup
touch "$SB/restore.fail"
run
assert_eq "restore error: exit" 1 "$rc"
assert_has "restore error: report" "$out" "FAIL: pg_restore reported errors"
assert_has "restore error: container removed" "$(calls)" "rm -f socialos-restore-test-"
setup
rm "$SB/bk"/*.dump
run
assert_eq "no dump: exit" 1 "$rc"
assert_has "no dump: report" "$out" "FAIL: no socialos-db-*.dump"
assert_lacks "no dump: nothing started" "$(calls)" "run -d"
setup
: >"$SB/bk/empty.dump"
run "$SB/bk/empty.dump"
assert_eq "empty dump: exit" 1 "$rc"

finish
