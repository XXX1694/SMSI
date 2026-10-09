#!/usr/bin/env bash
# The alert checks of socialos-guard.sh (sourced by it after its helpers alert, clear_alert and dk; not run on its own).
# They only report: a protected unit that is not active, a failing health URL, containers outside socialos.slice, a disk
# above GUARD_DISK_WARN, Steerpost data above GUARD_DATA_BUDGET_GB. None of them stops or starts anything.

check_health() {
  local unit url status name
  for unit in $PROTECTED_UNITS; do
    if systemctl is-active --quiet "$unit"; then clear_alert "$unit-inactive"; else
      alert "$unit-inactive" "$unit is not active (Steerpost does not manage it: systemctl status $unit)"
    fi
  done
  for url in $HEALTH_URLS; do
    name="url-$(printf '%s' "$url" | tr -c 'A-Za-z0-9.-' '_')"
    status=$(curl -sS -o /dev/null -w '%{http_code}' --max-time 10 "$url" 2>/dev/null || true)
    if [[ "$status" =~ ^[1-4][0-9][0-9]$ ]]; then clear_alert "$name"; else alert "$name" "$url answered ${status:-000}"; fi
  done
}

check_slice() {
  local max outside=() id parent
  max=$(systemctl show -p MemoryMax --value socialos.slice 2>/dev/null || true)
  if [ -z "$max" ] || [ "$max" = infinity ]; then
    alert slice "socialos.slice has no memory limit: is systemd/socialos.slice installed (README, Sharing a host safely)?"
    return 0
  fi
  for id in $(dk ps -q --filter "label=com.docker.compose.project=$PROJECT" 2>/dev/null || true); do
    parent=$(dk inspect -f '{{.HostConfig.CgroupParent}}' "$id" 2>/dev/null || true)
    if [ "$parent" != socialos.slice ]; then outside+=("$id"); fi
  done
  if [ "${#outside[@]}" -gt 0 ]; then
    alert slice "${#outside[@]} Steerpost container(s) run outside socialos.slice (${outside[*]}): docker compose up -d"
  else
    clear_alert slice
  fi
}

# check_budgets: the disk alert every run, the Steerpost data total (a du) at most once an hour. Alerts only: stopping
# containers frees no disk space.
check_budgets() {
  local used
  if ((DISK_PCT >= DISK_WARN)); then alert disk "disk ${DISK_PCT}% >= ${DISK_WARN}%"; else clear_alert disk; fi
  if [ -n "$(find "$STATE_DIR/data.checked" -mmin -60 2>/dev/null)" ]; then return 0; fi
  touch "$STATE_DIR/data.checked"
  used=$(data_used_gb)
  if ((used >= DATA_BUDGET_GB)); then
    alert data-budget "Steerpost data ${used} GB >= ${DATA_BUDGET_GB} GB (docker system df -v; MinIO, Postgres, backups)"
  else
    clear_alert data-budget
  fi
}
