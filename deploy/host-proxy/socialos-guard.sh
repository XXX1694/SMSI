#!/usr/bin/env bash
# Keep SocialOS from slowing down the other services on a shared host. Started every 2 minutes by
# systemd/socialos-guard.timer; also fine to run by hand (as root).
#
#   ./host-proxy/socialos-guard.sh             measure, alert, and shed SocialOS load only if the host is under pressure
#   ./host-proxy/socialos-guard.sh --dry-run   measure and say what it would do; changes nothing
#   ./host-proxy/socialos-guard.sh --status    the current shed state and alerts
#   ./host-proxy/socialos-guard.sh --resume    undo the shedding (unpause the worker, start the stopped services)
#
# Pressure means: little RAM left, swap almost full, tasks waiting for memory, a protected service (GUARD_PROTECTED_UNITS,
# default irbisa.service caddy.service) waiting for CPU, IO or memory, or a nearly full disk. Then, and only then:
#   level 1 (pressure)  pause the SocialOS worker (no publishing, no jobs); resumed by itself after 3 calm runs;
#   level 2 (critical, 2 runs in a row)  stop GUARD_SHED_SERVICES (default worker mcp frontend); stays so until --resume.
# Alerts (never an action): a protected unit is not active or a GUARD_HEALTH_URLS URL answers >= 500; the disk is above
# GUARD_DISK_WARN; SocialOS data above GUARD_DATA_BUDGET_GB; containers outside socialos.slice or the slice has no limits.
# Each alert is a file in .deploy/guard/alerts/ (removed when the condition clears) and a journal line:
#   journalctl -u socialos-guard -p warning
# It acts only on containers labelled com.docker.compose.project=socialos. It reads the protected units' state and cgroup
# files and never starts, stops or changes them. Settings: GUARD_* in the environment or in .env (guard-measure.sh).
# Exit codes: 0 done (also when it shed load); 1 an action failed (docker did not answer, ...); 2 bad settings.
set -Eeuo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
SOCIALOS_DIR=${SOCIALOS_DIR:-$(dirname "$script_dir")}
STATE_DIR=${GUARD_STATE_DIR:-$SOCIALOS_DIR/.deploy/guard}
DOCKER_TIMEOUT=${GUARD_DOCKER_TIMEOUT:-30}
PROJECT=socialos
# shellcheck source=guard-measure.sh
source "$script_dir/guard-measure.sh"

# Under systemd the journal reads a "<N>" prefix as the priority of the line.
P_WARN="" P_ERR=""
if [ -n "${JOURNAL_STREAM-}" ]; then P_WARN="<4>" P_ERR="<3>"; fi
log() { printf 'socialos-guard: %s\n' "$*"; }
warn() { printf '%ssocialos-guard: %s\n' "$P_WARN" "$*"; }
error() { printf '%ssocialos-guard: %s\n' "$P_ERR" "$*"; }
usage() { sed -n '2,20p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; }
FAILED=0

state_get() { cat "$STATE_DIR/$1" 2>/dev/null || true; }
state_set() { printf '%s\n' "$2" >"$STATE_DIR/$1"; }

# alert NAME MESSAGE / clear_alert NAME: a marker file that stays while the condition lasts. The journal line is written
# at every run while it lasts, so `journalctl -p warning` shows how long it lasted.
alert() {
  [ -e "$STATE_DIR/alerts/$1" ] || printf 'since %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >"$STATE_DIR/alerts/$1"
  printf '%s\n' "$2" >"$STATE_DIR/alerts/$1.last"
  warn "ALERT $1: $2"
}
clear_alert() {
  if [ -e "$STATE_DIR/alerts/$1" ]; then log "cleared: $1"; fi
  rm -f "$STATE_DIR/alerts/$1" "$STATE_DIR/alerts/$1.last"
}

# dk ARGS: docker with a time limit, so a hung daemon cannot hang the guard.
dk() { timeout "$DOCKER_TIMEOUT" docker "$@"; }
# ids SERVICE STATUS: ids of the SocialOS containers of SERVICE in STATUS (running, paused, exited).
ids() {
  dk ps -q --filter "label=com.docker.compose.project=$PROJECT" --filter "label=com.docker.compose.service=$1" \
    --filter "status=$2"
}
# each SERVICE STATUS ACTION: runs `docker ACTION <id>` for every matching container; failures are counted.
each() {
  local found id
  if ! found=$(ids "$1" "$2"); then
    error "docker did not answer (listing $1): cannot $3 it"
    FAILED=1
    return 0
  fi
  for id in $found; do
    if dk "$3" "$id" >/dev/null; then log "$3 $1 ($id)"; else
      error "docker $3 $1 ($id) failed"
      FAILED=1
    fi
  done
}

check_health() {
  local unit url status name
  for unit in $PROTECTED_UNITS; do
    if systemctl is-active --quiet "$unit"; then clear_alert "$unit-inactive"; else
      alert "$unit-inactive" "$unit is not active (SocialOS does not manage it: systemctl status $unit)"
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
    alert slice "${#outside[@]} SocialOS container(s) run outside socialos.slice (${outside[*]}): docker compose up -d"
  else
    clear_alert slice
  fi
}

# check_budgets: the disk alert every run, the SocialOS data total (a du) at most once an hour.
check_budgets() {
  local used
  if ((DISK_PCT >= DISK_WARN)); then alert disk "disk ${DISK_PCT}% >= ${DISK_WARN}%"; else clear_alert disk; fi
  if [ -n "$(find "$STATE_DIR/data.checked" -mmin -60 2>/dev/null)" ]; then return 0; fi
  touch "$STATE_DIR/data.checked"
  used=$(data_used_gb)
  if ((used >= DATA_BUDGET_GB)); then
    alert data-budget "SocialOS data ${used} GB >= ${DATA_BUDGET_GB} GB (docker system df -v; MinIO, Postgres, backups)"
  else
    clear_alert data-budget
  fi
}

shed() { # LEVEL
  local svc
  if [ "$1" -ge 2 ]; then
    for svc in $SHED_SERVICES; do
      each "$svc" paused unpause # a paused container is unpaused so that it can stop cleanly
      each "$svc" running stop
    done
  else
    each worker running pause
  fi
}

resume() {
  local svc
  each worker paused unpause
  if [ "$(state_get shed)" = 2 ]; then
    for svc in $SHED_SERVICES; do each "$svc" exited start; done
  fi
  rm -f "$STATE_DIR/shed" "$STATE_DIR/crit_runs"
  clear_alert shed
  log "resumed"
}

# decide: the level to act on. A critical reading stops containers only when it repeats (CRIT_RUNS in a row).
decide() {
  local runs
  if [ "$LEVEL" -lt 2 ]; then
    state_set crit_runs 0
    printf '%s' "$LEVEL"
    return 0
  fi
  runs=$(state_get crit_runs)
  runs=$((${runs:-0} + 1))
  state_set crit_runs "$runs"
  if [ "$runs" -ge "$CRIT_RUNS" ]; then printf 2; else printf 1; fi
}

act() {
  local level shed_now calm
  level=$(decide)
  shed_now=$(state_get shed)
  if [ "$level" -gt 0 ]; then
    state_set calm_runs 0
    alert pressure "level $level: ${REASONS[*]}"
    shed "$level"
    if [ "$level" -gt "${shed_now:-0}" ]; then state_set shed "$level"; fi
    alert shed "SocialOS load shed at level $(state_get shed) (1: worker paused, 2: $SHED_SERVICES stopped). Undo: socialos-guard.sh --resume"
    return 0
  fi
  clear_alert pressure
  calm=$(state_get calm_runs)
  calm=$((${calm:-0} + 1))
  state_set calm_runs "$calm"
  if [ "$shed_now" = 1 ] && [ "$calm" -ge "$RESUME_AFTER" ]; then
    log "calm for $calm runs: resuming the worker"
    resume
  elif [ "$shed_now" = 2 ]; then
    warn "the host is calm, but $SHED_SERVICES stay stopped until someone runs: socialos-guard.sh --resume"
  fi
}

status() {
  local f
  echo "shed level: $(state_get shed) (state in $STATE_DIR)"
  for f in "$STATE_DIR"/alerts/*; do
    case "$f" in *.last | *'*') continue ;; esac
    printf '%s: %s; %s\n' "${f##*/}" "$(cat "$f")" "$(cat "$f.last" 2>/dev/null || true)"
  done
}

mode=run
case "${1-}" in
  "") ;;
  --dry-run) mode=dry ;;
  --status) mode=status ;;
  --resume) mode=resume ;;
  -h | --help)
    usage
    exit 0
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac

load_settings
if [ "$mode" = status ]; then
  status
  exit 0
fi
assess
if [ "$mode" = dry ]; then
  log "level $LEVEL${REASONS[*]:+: ${REASONS[*]}} (disk ${DISK_PCT}%); dry run, nothing changed"
  exit 0
fi

mkdir -p "$STATE_DIR/alerts"
exec 8>"$STATE_DIR/lock"
flock -w 60 8 || {
  error "another run holds $STATE_DIR/lock"
  exit 1
}
if [ "$mode" = resume ]; then resume; else
  check_health
  check_slice
  check_budgets
  act
  log "level $LEVEL${REASONS[*]:+: ${REASONS[*]}} (disk ${DISK_PCT}%)"
fi
exit "$FAILED"
