#!/usr/bin/env bash
# Dump the SocialOS database (and optionally the uploaded media) and delete old backups.
#
#   ./backup.sh                          # database only
#   BACKUP_MEDIA=1 ./backup.sh           # also archive the bundled MinIO volume (skip when you use external S3)
#
# Settings (environment): BACKUP_DIR (default /var/backups/socialos), KEEP_DAYS (default 14).
# Cron (as the deploy user, daily 03:17):
#   17 3 * * * /opt/socialos/backup.sh >>/var/log/socialos-backup.log 2>&1
# Also keep a copy of ./.env somewhere safe: the dump contains OAuth tokens encrypted with ENCRYPTION_KEY.
# Restore: see README.md ("Backups").
set -Eeuo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"

BACKUP_DIR=${BACKUP_DIR:-/var/backups/socialos}
KEEP_DAYS=${KEEP_DAYS:-14}
BACKUP_MEDIA=${BACKUP_MEDIA:-0}
COMPOSE_YML=docker-compose.prod.yml

umask 077
mkdir -p "$BACKUP_DIR"
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
mv "$dump.partial" "$dump"
echo "database dump: $dump ($(du -h "$dump" | cut -f1))"

if [ "$BACKUP_MEDIA" = 1 ]; then
  volume=$(docker volume ls --format '{{.Name}}' | grep -E '(^|_)minio-data$' | head -n 1 || true)
  if [ -n "$volume" ]; then
    media="$BACKUP_DIR/socialos-media-$stamp.tar.gz"
    docker run --rm -v "$volume":/data:ro -v "$BACKUP_DIR":/backup alpine:3 \
      tar -czf "/backup/socialos-media-$stamp.tar.gz" -C /data .
    echo "media archive: $media ($(du -h "$media" | cut -f1))"
  else
    echo "BACKUP_MEDIA=1 but no minio-data volume was found; skipped" >&2
  fi
fi

find "$BACKUP_DIR" -maxdepth 1 -type f \( -name 'socialos-db-*.dump' -o -name 'socialos-media-*.tar.gz' \) -mtime "+$KEEP_DAYS" -print -delete
