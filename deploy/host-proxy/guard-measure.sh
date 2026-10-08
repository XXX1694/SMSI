#!/usr/bin/env bash
# Settings and measurements of socialos-guard.sh (sourced by it, not run on its own). Reads only: /proc, the cgroup
# files of the protected units, df, du and `systemctl is-active` / `systemctl show`. It never changes anything.
# shellcheck disable=SC2034 # the settings are read by socialos-guard.sh

PROC_DIR=${PROC_DIR:-/proc}
CGROUP_ROOT=${CGROUP_ROOT:-/sys/fs/cgroup}

# env_file_get KEY: last assignment of KEY in $SOCIALOS_DIR/.env, without surrounding quotes (empty if none).
env_file_get() {
  local line
  [ -r "$SOCIALOS_DIR/.env" ] || return 0
  line=$(grep -E "^[[:space:]]*$1=" "$SOCIALOS_DIR/.env" | tail -n 1 || true)
  line=${line#*=}
  line=${line%\"}
  line=${line#\"}
  printf '%s' "$line"
}

# cfg KEY DEFAULT: the environment wins over .env, .env over the default.
cfg() {
  local val=${!1-}
  [ -n "$val" ] || val=$(env_file_get "$1")
  printf '%s' "${val:-$2}"
}

# cfg_int KEY DEFAULT: same, and it must be a whole number.
cfg_int() {
  local val
  val=$(cfg "$1" "$2")
  [[ "$val" =~ ^[0-9]+$ ]] || {
    printf 'socialos-guard: %s must be a whole number, got "%s"\n' "$1" "$val" >&2
    exit 2
  }
  printf '%s' "$val"
}

load_settings() {
  PROTECTED_UNITS=$(cfg GUARD_PROTECTED_UNITS "irbisa.service caddy.service")
  HEALTH_URLS=$(cfg GUARD_HEALTH_URLS "")
  SHED_SERVICES=$(cfg GUARD_SHED_SERVICES "worker mcp frontend")
  DISK_PATH=$(cfg GUARD_DISK_PATH "/")
  DATA_PATHS=$(cfg GUARD_DATA_PATHS "/var/lib/docker /var/lib/containerd /var/backups/socialos $SOCIALOS_DIR")
  MEM_AVAIL_WARN=$(cfg_int GUARD_MEM_AVAIL_WARN 15)   # % of RAM still available
  MEM_AVAIL_CRIT=$(cfg_int GUARD_MEM_AVAIL_CRIT 8)
  SWAP_USED_WARN=$(cfg_int GUARD_SWAP_USED_WARN 80)   # % of swap in use
  HOST_MEM_PSI_WARN=$(cfg_int GUARD_HOST_MEM_PSI_WARN 10) # % of the last minute some task waited for memory
  HOST_MEM_PSI_CRIT=$(cfg_int GUARD_HOST_MEM_PSI_CRIT 30)
  UNIT_PSI_WARN=$(cfg_int GUARD_UNIT_PSI_WARN 20)     # same, for the protected units (cpu, io, memory)
  UNIT_PSI_CRIT=$(cfg_int GUARD_UNIT_PSI_CRIT 50)
  DISK_WARN=$(cfg_int GUARD_DISK_WARN 80)             # % of the disk in use: alert
  DISK_CRIT=$(cfg_int GUARD_DISK_CRIT 92)             # ... and shed (stop the writers)
  DATA_BUDGET_GB=$(cfg_int GUARD_DATA_BUDGET_GB 15)   # SocialOS data on disk: alert above this
  CRIT_RUNS=$(cfg_int GUARD_CRIT_RUNS 2)              # consecutive critical runs before containers are stopped
  RESUME_AFTER=$(cfg_int GUARD_RESUME_AFTER 3)        # consecutive calm runs before a paused worker is resumed
}

LEVEL=0      # 0 calm, 1 pressure (pause the worker), 2 critical (stop the shed services)
REASONS=()

raise() { # LEVEL REASON
  if (($1 > LEVEL)); then LEVEL=$1; fi
  REASONS+=("$2")
}

# grade WHAT VALUE WARN CRIT: higher is worse.
grade() {
  if (($2 >= $4)); then
    raise 2 "$1 $2% >= $4%"
  elif (($2 >= $3)); then
    raise 1 "$1 $2% >= $3%"
  fi
}

meminfo_kb() { awk -v k="$1:" '$1 == k { print $2 }' "$PROC_DIR/meminfo"; }

# psi_avg60 FILE: the "some avg60" value of a pressure file as a whole number (0 when the file is missing).
psi_avg60() {
  local v=""
  if [ -r "$1" ]; then
    v=$(awk '$1 == "some" { for (i = 2; i <= NF; i++) if ($i ~ /^avg60=/) { sub(/^avg60=/, "", $i); print int($i) } }' "$1")
  fi
  printf '%s' "${v:-0}"
}

disk_used_pct() { df -P "$DISK_PATH" | awk 'NR == 2 { sub(/%/, "", $5); print $5 }'; }

assess_memory() {
  local total avail pct swap_total swap_free
  total=$(meminfo_kb MemTotal)
  avail=$(meminfo_kb MemAvailable)
  pct=$((avail * 100 / total))
  if ((pct < MEM_AVAIL_CRIT)); then
    raise 2 "available memory ${pct}% < ${MEM_AVAIL_CRIT}%"
  elif ((pct < MEM_AVAIL_WARN)); then
    raise 1 "available memory ${pct}% < ${MEM_AVAIL_WARN}%"
  fi
  swap_total=$(meminfo_kb SwapTotal)
  swap_free=$(meminfo_kb SwapFree)
  if ((${swap_total:-0} > 0)); then
    pct=$(((swap_total - swap_free) * 100 / swap_total))
    if ((pct >= SWAP_USED_WARN)); then raise 1 "swap in use ${pct}% >= ${SWAP_USED_WARN}%"; fi
  fi
  grade "host memory stall" "$(psi_avg60 "$PROC_DIR/pressure/memory")" "$HOST_MEM_PSI_WARN" "$HOST_MEM_PSI_CRIT"
}

# assess_units: how long the protected services waited for CPU, IO or memory in the last minute (their own PSI files).
# This is the signal that matters: it says whether those services are being slowed down, whoever causes it.
assess_units() {
  local unit res
  for unit in $PROTECTED_UNITS; do
    for res in cpu io memory; do
      grade "$unit $res stall" "$(psi_avg60 "$CGROUP_ROOT/system.slice/$unit/$res.pressure")" "$UNIT_PSI_WARN" "$UNIT_PSI_CRIT"
    done
  done
}

# assess: sets LEVEL and REASONS from the current host state.
assess() {
  local disk
  LEVEL=0
  REASONS=()
  assess_memory
  assess_units
  disk=$(disk_used_pct)
  if ((disk >= DISK_CRIT)); then raise 2 "disk ${disk}% >= ${DISK_CRIT}%"; fi
  DISK_PCT=$disk
}

# data_used_gb: SocialOS data on disk (images, volumes, logs, backups, /opt/socialos), whole GB. -x: stays on the
# filesystem, so the overlay mounts of running containers are not counted twice.
data_used_gb() {
  local kb
  # shellcheck disable=SC2086 # DATA_PATHS is a space-separated list on purpose
  kb=$({ nice -n 19 du -sxk $DATA_PATHS 2>/dev/null || true; } | awk '{ s += $1 } END { print s + 0 }')
  printf '%s' $((kb / 1024 / 1024))
}
