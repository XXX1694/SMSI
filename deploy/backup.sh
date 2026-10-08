#!/usr/bin/env bash
# Dump the SocialOS database (and optionally the uploaded media), delete old backups, optionally copy off-site.
#
#   ./backup.sh                          # database only
#   BACKUP_MEDIA=1 ./backup.sh           # also archive the bundled MinIO volume (skip when you use external S3)
#
# Settings (environment, or .env next to this script): BACKUP_DIR (default /var/backups/socialos), KEEP_DAYS (default 14).
# Every artifact is created mode 600 and the directory mode 700: the dump holds OAuth tokens and the media are user uploads.
#
# Off-site copy (optional; skipped with one info line unless all four are set, README section 9):
#   BACKUP_S3_URL=s3://bucket/prefix  BACKUP_S3_ACCESS_KEY  BACKUP_S3_SECRET_KEY  BACKUP_AGE_RECIPIENT=age1...
#   BACKUP_S3_ENDPOINT=https://...  (B2/R2/MinIO; empty = AWS)
# The new artifacts are encrypted with `age` for the public recipient (the private key never lives here) and uploaded with
# rclone; both run in containers pinned by digest. Keys are passed to the containers through the environment, never logged.
#
# RESTORE_TEST=1|weekly also runs restore-test.sh on the new dump (weekly = on Sundays only); a failed drill fails the run.
# Schedule: the systemd timer in systemd/socialos-backup.timer (README section 9). Restore: README section 9, restore-test.sh.
# Also keep a copy of ./.env somewhere safe: the dump contains OAuth tokens encrypted with ENCRYPTION_KEY.
set -Eeuo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"

BACKUP_DIR=${BACKUP_DIR:-/var/backups/socialos}
KEEP_DAYS=${KEEP_DAYS:-14}
BACKUP_MEDIA=${BACKUP_MEDIA:-0}
COMPOSE_YML=docker-compose.prod.yml

# A pinned image per job: rclone uploads, a pinned Alpine runs age (the age package is installed from Alpine's signed repo).
RCLONE_IMAGE=${RCLONE_IMAGE:-rclone/rclone:1.68.2@sha256:74c51b8817e5431bd6d7ed27cb2a50d8ee78d77f6807b72a41ef6f898845942b}
AGE_IMAGE=${AGE_IMAGE:-alpine:3.22@sha256:5291449c3df73caf6ed85e649dec1b9e818b39a5d8c871e97afc13e9cd5e8fa8}

# cfg NAME: the variable from the environment, else its last assignment in .env (quotes stripped), else empty.
cfg() {
  local line=${!1-}
  if [ -z "$line" ] && [ -f .env ]; then
    line=$(grep -E "^[[:space:]]*$1=" .env | tail -n 1 || true)
    line=${line#*=}
    line=${line%\"}
    line=${line#\"}
    line=${line%\'}
    line=${line#\'}
  fi
  printf '%s' "$line"
}

umask 077
install -d -m 700 "$BACKUP_DIR"
chmod 700 "$BACKUP_DIR"
stamp=$(date -u +%Y%m%dT%H%M%SZ)
dump="$BACKUP_DIR/socialos-db-$stamp.dump"

# Custom format (-Fc): compressed, restorable with pg_restore, selective. The credentials come from the container.
docker compose -f "$COMPOSE_YML" exec -T postgres \
  sh -c 'exec pg_dump --username="$POSTGRES_USER" --dbname="$POSTGRES_DB" --format=custom --no-owner' >"$dump.partial"
[ -s "$dump.partial" ] || {
  echo "backup failed: empty dump" >&2
  rm -f "$dump.partial"
  exit 1
}
chmod 600 "$dump.partial"
mv "$dump.partial" "$dump"
artifacts=("$dump")
echo "database dump: $dump ($(du -h "$dump" | cut -f1))"

if [ "$BACKUP_MEDIA" = 1 ]; then
  volume=$(docker volume ls --format '{{.Name}}' | grep -E '(^|_)minio-data$' | head -n 1 || true)
  if [ -n "$volume" ]; then
    media="$BACKUP_DIR/socialos-media-$stamp.tar.gz"
    # The archive is streamed to the host and written by this shell (umask 077), not by tar inside the container (umask 022).
    docker run --rm -v "$volume":/data:ro alpine:3 tar -czf - -C /data . >"$media.partial"
    chmod 600 "$media.partial"
    mv "$media.partial" "$media"
    artifacts+=("$media")
    echo "media archive: $media ($(du -h "$media" | cut -f1))"
  else
    echo "BACKUP_MEDIA=1 but no minio-data volume was found; skipped" >&2
  fi
fi

find "$BACKUP_DIR" -maxdepth 1 -type f \( -name 'socialos-db-*.dump' -o -name 'socialos-media-*.tar.gz' -o -name '*.age' -o -name '*.partial' \) -mtime "+$KEEP_DAYS" -print -delete

# ---- restore drill (weekly, from the same timer) -------------------------------------------------------------------
# RESTORE_TEST=1 restores the new dump into a throwaway container after every backup; RESTORE_TEST=weekly only on Sundays.
drill_rc=0
if [ "${RESTORE_TEST:-0}" = 1 ] || { [ "${RESTORE_TEST:-0}" = weekly ] && [ "$(date +%u)" = 7 ]; }; then
  ./restore-test.sh "$dump" || drill_rc=$?
fi

# ---- off-site copy -------------------------------------------------------------------------------------------------
offsite() {
  s3_url=$(cfg BACKUP_S3_URL)
  s3_key=$(cfg BACKUP_S3_ACCESS_KEY)
  s3_secret=$(cfg BACKUP_S3_SECRET_KEY)
  age_recipient=$(cfg BACKUP_AGE_RECIPIENT)
  if [ -z "$s3_url" ] || [ -z "$s3_key" ] || [ -z "$s3_secret" ] || [ -z "$age_recipient" ]; then
    echo "off-site copy: skipped (BACKUP_S3_URL, BACKUP_S3_ACCESS_KEY, BACKUP_S3_SECRET_KEY and BACKUP_AGE_RECIPIENT are not all set)"
    return 0
  fi
  case $s3_url in
    s3://?*) ;;
    *)
      echo "off-site copy: BACKUP_S3_URL must look like s3://bucket/prefix" >&2
      exit 1
      ;;
  esac
  remote_path=${s3_url#s3://}
  remote_path=${remote_path%/}

  # rclone reads its remote "off" from RCLONE_CONFIG_OFF_* variables; the secrets go in by name (-e NAME), so they never
  # appear in a process list or in this script's output.
  export RCLONE_CONFIG_OFF_TYPE=s3
  export RCLONE_CONFIG_OFF_PROVIDER=Other
  export RCLONE_CONFIG_OFF_ACCESS_KEY_ID=$s3_key
  export RCLONE_CONFIG_OFF_SECRET_ACCESS_KEY=$s3_secret
  endpoint=$(cfg BACKUP_S3_ENDPOINT)
  if [ -n "$endpoint" ]; then
    export RCLONE_CONFIG_OFF_ENDPOINT=$endpoint
  else
    export RCLONE_CONFIG_OFF_PROVIDER=AWS
  fi
  export AGE_RECIPIENT=$age_recipient

  for f in "${artifacts[@]}"; do
    name=$(basename "$f")
    enc="$f.age"
    docker run --rm -i -e AGE_RECIPIENT "$AGE_IMAGE" \
      sh -c 'apk add -q --no-cache age >&2 && exec age -r "$AGE_RECIPIENT" -' <"$f" >"$enc.partial" || {
      rm -f "$enc.partial"
      echo "off-site copy: encrypting $name failed" >&2
      exit 1
    }
    chmod 600 "$enc.partial"
    mv "$enc.partial" "$enc"
    docker run --rm -v "$BACKUP_DIR":/in:ro \
      -e RCLONE_CONFIG_OFF_TYPE -e RCLONE_CONFIG_OFF_PROVIDER -e RCLONE_CONFIG_OFF_ENDPOINT \
      -e RCLONE_CONFIG_OFF_ACCESS_KEY_ID -e RCLONE_CONFIG_OFF_SECRET_ACCESS_KEY \
      "$RCLONE_IMAGE" copyto "/in/$name.age" "off:$remote_path/$name.age" || {
      echo "off-site copy: uploading $name.age failed (the local backup is intact; the encrypted copy stays in $BACKUP_DIR)" >&2
      exit 1
    }
    rm -f "$enc"
    echo "off-site copy: $name.age -> s3://$remote_path/"
  done
}

offsite
exit "$drill_rc"
