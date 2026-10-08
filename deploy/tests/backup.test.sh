#!/usr/bin/env bash
# backup.sh against a stubbed `docker`: file modes (700 dir, 600 files), the optional encrypted off-site copy (skip, upload,
# no secrets in output or arguments, failures) and the weekly restore drill hook. Needs Linux (stat -c).
# shellcheck source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

# setup [ENV_LINES]: sandbox with backup.sh, a .env, a stub restore-test.sh and a stub docker. The stub answers:
#   compose ... exec -T postgres    -> "PGDUMP" (the dump)          volume ls -> socialos_minio-data
#   run ... alpine:3 tar            -> "TARDATA" on stdout (the media archive, written 644 by a real container)
#   run ... alpine:3.22 (age)       -> stdin prefixed with "AGE:"   run ... rclone (upload) -> fails when $SB/upload.fail exists
# Every call goes to $SB/calls; the secrets the rclone container would receive in its environment go to $SB/rclone.env.
setup() {
  new_sb
  mkdir -p "$SB/app" "$SB/bk"
  cp "$REPO_DEPLOY/backup.sh" "$SB/app/"
  printf '%s\n' "${1-}" >"$SB/app/.env"
  cat >"$SB/app/restore-test.sh" <<STUB
#!/usr/bin/env bash
echo "\$*" >>"$SB/drill.calls"
exit "\$(cat "$SB/drill.rc" 2>/dev/null || echo 0)"
STUB
  cat >"$SB/bin/docker" <<STUB
#!/usr/bin/env bash
echo "\$*" >>"$SB/calls"
case "\$*" in
  "compose -f docker-compose.prod.yml exec -T postgres"*) printf PGDUMP ;;
  "volume ls"*) echo socialos_minio-data ;;
  *"alpine:3 tar"*) printf TARDATA ;;
  *alpine:3.22*) printf 'AGE:'; cat ;;
  *rclone*)
    [ ! -e "$SB/upload.fail" ] || exit 1
    echo "key=\${RCLONE_CONFIG_OFF_ACCESS_KEY_ID-} secret=\${RCLONE_CONFIG_OFF_SECRET_ACCESS_KEY-} endpoint=\${RCLONE_CONFIG_OFF_ENDPOINT-} provider=\${RCLONE_CONFIG_OFF_PROVIDER-}" >>"$SB/rclone.env"
    ;;
esac
exit 0
STUB
  chmod +x "$SB/bin/docker" "$SB/app/restore-test.sh"
}
run() { # run [ENV=VALUE...]: sets $out and $rc
  rc=0
  out=$(cd "$SB/app" && env PATH="$SB/bin:$PATH" BACKUP_DIR="$SB/bk" "$@" bash ./backup.sh 2>&1) || rc=$?
}
mode() { stat -c %a "$1"; }
calls() { cat "$SB/calls" 2>/dev/null || true; }
dumpfile() { find "$SB/bk" -name 'socialos-db-*.dump' | head -n 1; }

# 1. database only: dump mode 600, directory 700 (an existing 755 directory is tightened), off-site skipped with one info line
setup
chmod 755 "$SB/bk"
run
assert_eq "db only: exit" 0 "$rc"
assert_eq "db only: directory mode" 700 "$(mode "$SB/bk")"
assert_eq "db only: dump mode" 600 "$(mode "$(dumpfile)")"
assert_eq "db only: dump content" PGDUMP "$(cat "$(dumpfile)")"
assert_eq "db only: no media archive" 0 "$(find "$SB/bk" -name '*media*' | wc -l)"
assert_eq "off-site skipped: one info line" 1 "$(grep -c 'off-site copy: skipped' <<<"$out")"
assert_lacks "off-site skipped: no rclone" "$(calls)" rclone
assert_lacks "off-site skipped: no age" "$(calls)" alpine:3.22
assert_no_file "no drill by default" "$SB/drill.calls"

# 2. a new directory is created 700 even under a permissive umask
setup
rmdir "$SB/bk"
run
assert_eq "new directory: mode" 700 "$(mode "$SB/bk")"

# 3. media archive: mode 600 although the container's tar would have written 644, no leftovers
setup
run BACKUP_MEDIA=1
media=$(find "$SB/bk" -name 'socialos-media-*.tar.gz' | head -n 1)
assert_eq "media: exit" 0 "$rc"
assert_eq "media: mode" 600 "$(mode "$media")"
assert_eq "media: content" TARDATA "$(cat "$media")"
assert_eq "media: no partial files" 0 "$(find "$SB/bk" -name '*.partial' | wc -l)"
assert_has "media: reported" "$out" "media archive:"

# 4. off-site copy from the environment: encrypted, uploaded, secrets only in the container environment
setup
run BACKUP_MEDIA=1 BACKUP_S3_URL=s3://bkt/pre/ BACKUP_S3_ACCESS_KEY=AKIAEXAMPLE BACKUP_S3_SECRET_KEY=sEcReT-value \
  BACKUP_S3_ENDPOINT=https://s3.example.test BACKUP_AGE_RECIPIENT=age1examplepublickey
assert_eq "off-site: exit" 0 "$rc"
assert_has "off-site: dump uploaded" "$(calls)" "copyto /in/$(basename "$(dumpfile)").age off:bkt/pre/$(basename "$(dumpfile)").age"
assert_has "off-site: media uploaded" "$(calls)" "off:bkt/pre/socialos-media-"
assert_has "off-site: rclone pinned by digest" "$(calls)" "rclone/rclone:1.68.2@sha256:"
assert_has "off-site: age pinned by digest" "$(calls)" "alpine:3.22@sha256:"
assert_has "off-site: recipient reaches age by name" "$(calls)" "-e AGE_RECIPIENT"
assert_lacks "off-site: no secret in docker arguments" "$(calls)" sEcReT-value
assert_lacks "off-site: no access key in docker arguments" "$(calls)" AKIAEXAMPLE
assert_lacks "off-site: no secret in output" "$out" sEcReT-value
assert_lacks "off-site: no access key in output" "$out" AKIAEXAMPLE
assert_has "off-site: secrets reach the container environment" "$(cat "$SB/rclone.env")" "key=AKIAEXAMPLE secret=sEcReT-value endpoint=https://s3.example.test provider=Other"
assert_eq "off-site: local .age copies removed" 0 "$(find "$SB/bk" -name '*.age' | wc -l)"
assert_eq "off-site: plain backups kept" 600 "$(mode "$(dumpfile)")"

# 5. configuration from .env, no endpoint = AWS
setup 'BACKUP_S3_URL="s3://bkt"
BACKUP_S3_ACCESS_KEY=k1
BACKUP_S3_SECRET_KEY=s1
BACKUP_AGE_RECIPIENT=age1x'
run
assert_eq ".env: exit" 0 "$rc"
assert_has ".env: uploaded" "$(calls)" "off:bkt/socialos-db-"
assert_has ".env: AWS provider without an endpoint" "$(cat "$SB/rclone.env")" "endpoint= provider=AWS"

# 6. incomplete settings: skipped, not an error
setup 'BACKUP_S3_URL=s3://bkt
BACKUP_S3_ACCESS_KEY=k1'
run
assert_eq "partial settings: exit" 0 "$rc"
assert_has "partial settings: skipped" "$out" "off-site copy: skipped"

# 7. bad URL and failed upload: exit 1, the local backup stays
setup
run BACKUP_S3_URL=http://nope BACKUP_S3_ACCESS_KEY=k BACKUP_S3_SECRET_KEY=s BACKUP_AGE_RECIPIENT=age1x
assert_eq "bad URL: exit" 1 "$rc"
assert_has "bad URL: message" "$out" "must look like s3://bucket/prefix"
assert_file "bad URL: local dump kept" "$(dumpfile)"
setup
touch "$SB/upload.fail"
run BACKUP_S3_URL=s3://bkt BACKUP_S3_ACCESS_KEY=k BACKUP_S3_SECRET_KEY=s BACKUP_AGE_RECIPIENT=age1x
assert_eq "upload failure: exit" 1 "$rc"
assert_has "upload failure: message" "$out" "uploading"
assert_file "upload failure: local dump kept" "$(dumpfile)"
assert_eq "upload failure: encrypted copy kept for the next look" 600 "$(mode "$(find "$SB/bk" -name '*.age' | head -n 1)")"

# 8. the restore drill: RESTORE_TEST=1 always, =weekly only on Sundays, a failed drill fails the run
setup
run RESTORE_TEST=1
assert_eq "drill=1: exit" 0 "$rc"
assert_has "drill=1: runs on the new dump" "$(cat "$SB/drill.calls")" "$(basename "$(dumpfile)")"
setup
echo 3 >"$SB/drill.rc"
run RESTORE_TEST=1
assert_eq "drill fails: exit is the drill's" 3 "$rc"
for day in 7 3; do
  setup
  cat >"$SB/bin/date" <<STUB
#!/usr/bin/env bash
if [ "\$*" = "+%u" ]; then echo $day; else exec /usr/bin/date "\$@"; fi
STUB
  chmod +x "$SB/bin/date"
  run RESTORE_TEST=weekly
  if [ "$day" = 7 ]; then assert_file "drill=weekly on Sunday" "$SB/drill.calls"; else assert_no_file "drill=weekly on Wednesday" "$SB/drill.calls"; fi
done

finish
