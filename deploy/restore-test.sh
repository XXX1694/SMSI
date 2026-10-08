#!/usr/bin/env bash
# Restore drill: restore the newest database dump (or the file given) into a throwaway postgres container that has no
# network, then compare it with the live database. Prints a short PASS/FAIL report; the container is always removed.
#
#   ./restore-test.sh                       # newest socialos-db-*.dump in BACKUP_DIR (default /var/backups/socialos)
#   ./restore-test.sh /path/to/file.dump
#
# Run as root (or a user that can use Docker) on the server, from /opt/socialos. Exit code 0 = PASS, 1 = FAIL.
# What it proves: the dump restores without errors, has as many tables as the live database and the same goose version.
# It does not prove the data is complete; the row counts are printed so that you can eyeball them.
set -Eeuo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"

BACKUP_DIR=${BACKUP_DIR:-/var/backups/socialos}
PG_IMAGE=${RESTORE_PG_IMAGE:-postgres:16-alpine}
COMPOSE_YML=docker-compose.prod.yml
name="socialos-restore-test-$$"

# shellcheck disable=SC2329 # runs from the trap
cleanup() { docker rm -f "$name" >/dev/null 2>&1 || true; }
trap cleanup EXIT

fail() {
  echo "restore test: FAIL: $*"
  exit 1
}

dump=${1-}
if [ -z "$dump" ]; then
  dump=$(find "$BACKUP_DIR" -maxdepth 1 -type f -name 'socialos-db-*.dump' 2>/dev/null | sort | tail -n 1)
  [ -n "$dump" ] || fail "no socialos-db-*.dump in $BACKUP_DIR"
fi
[ -s "$dump" ] || fail "$dump is missing or empty"
dump=$(cd "$(dirname "$dump")" && pwd)/$(basename "$dump")

# q SQL: run on the throwaway database / on the live one; one value, no decoration.
q_tmp() { docker exec "$name" psql -U postgres -d restoretest -tA -c "$1"; }
q_live() {
  docker compose -f "$COMPOSE_YML" exec -T postgres \
    sh -c 'exec psql --username="$POSTGRES_USER" --dbname="$POSTGRES_DB" -tA -c "$1"' sh "$1"
}
tables_sql="select count(*) from information_schema.tables where table_schema='public' and table_type='BASE TABLE'"
goose_sql="select coalesce(max(version_id),0) from goose_db_version where is_applied"

# --network none: the restored data (OAuth tokens, e-mail addresses) can never leave the box, and nothing can connect in.
docker run -d --name "$name" --network none -e POSTGRES_PASSWORD=throwaway \
  "$PG_IMAGE" >/dev/null || fail "could not start $PG_IMAGE"

ready=0
for _ in $(seq 1 90); do
  # The image starts a temporary server for its init and then restarts: ready twice means the real one is up.
  if [ "$(docker logs "$name" 2>&1 | grep -c 'database system is ready to accept connections')" -ge 2 ]; then
    ready=1
    break
  fi
  sleep 1
done
[ "$ready" = 1 ] || fail "the throwaway database did not become ready in 90 s"

docker exec "$name" createdb -U postgres restoretest || fail "createdb failed"
# Streamed through stdin: the dump is mode 600 root, the postgres user inside the container could not read a bind mount of it.
docker exec -i "$name" pg_restore -U postgres -d restoretest --no-owner --exit-on-error <"$dump" ||
  fail "pg_restore reported errors"

tmp_tables=$(q_tmp "$tables_sql") || fail "cannot count tables in the restored copy"
tmp_goose=$(q_tmp "$goose_sql") || fail "no goose_db_version in the restored copy"
live_tables=$(q_live "$tables_sql") || fail "cannot count tables in the live database (is the stack up?)"
live_goose=$(q_live "$goose_sql") || fail "cannot read goose_db_version in the live database"
tmp_tables=${tmp_tables//[$'\r\n ']/}
live_tables=${live_tables//[$'\r\n ']/}
tmp_goose=${tmp_goose//[$'\r\n ']/}
live_goose=${live_goose//[$'\r\n ']/}

echo "restore test: dump    $(basename "$dump")"
echo "restore test: tables  restored=$tmp_tables live=$live_tables"
echo "restore test: goose   restored=$tmp_goose live=$live_goose"
[ "$tmp_tables" = "$live_tables" ] || fail "table count differs"
[ "$tmp_goose" = "$live_goose" ] || fail "goose version differs (the dump is older or newer than the live schema)"
[ "$tmp_tables" -gt 0 ] || fail "the restored database has no tables"
echo "restore test: PASS"
