#!/usr/bin/env bash
# Keep SocialOS from slowing down the other services on a shared host. Started every 2 minutes by
# systemd/socialos-guard.timer; also fine to run by hand (as root).
#
#   ./host-proxy/socialos-guard.sh             measure, alert, and shed SocialOS load if SocialOS adds to host pressure
#   ./host-proxy/socialos-guard.sh --dry-run   measure and say what it would do; changes nothing
#   ./host-proxy/socialos-guard.sh --status    the current shed state and alerts
#   ./host-proxy/socialos-guard.sh --resume    undo the shedding now (start what the guard stopped)
#
# Pressure: little RAM left, swap almost full, tasks waiting for memory, or a protected service (GUARD_PROTECTED_UNITS,
# default irbisa.service caddy.service) waiting for CPU, IO or memory. The guard acts only when SocialOS is also a real
# contributor (socialos.slice anonymous memory, page cache excluded, above GUARD_SLICE_MEM_MB, GUARD_SLICE_CPU_PCT or GUARD_SLICE_IO_MBPS); otherwise it alerts.
#   level 1 (pressure)  stop the SocialOS worker gracefully (its stop grace period); no publishing, no jobs;
#   level 2 (critical, 2 runs in a row)  also stop GUARD_SHED_SERVICES (default worker mcp frontend).
# It starts again only what it stopped, after GUARD_RESUME_AFTER calm runs (level 1) or GUARD_RESUME_AFTER_CRIT calm runs
# doubled for each level 2 within a day (level 2). Alerts (never an action): a protected unit is not active or a
# GUARD_HEALTH_URLS URL answers >= 500; the disk is above GUARD_DISK_WARN; SocialOS data above GUARD_DATA_BUDGET_GB;
# containers outside socialos.slice. Each alert is a file in .deploy/guard/alerts/ and a journal line:
#   journalctl -u socialos-guard -p warning
# A container that thrashes its own page cache (memory at GUARD_THRASH_MEM_PCT of its cap and refaults above
# GUARD_THRASH_REFAULT_MBPS) is the cause no shedding can fix: the guard raises the alert thrash-<service>, restarts it
# (GUARD_THRASH_ESSENTIAL: postgres redis minio backend) or stops it (the others), once per GUARD_THRASH_COOLDOWN, and
# skips shedding that run. The pressure alert also names an IO-heavy container that shedding would not stop.
# It acts only on containers labelled com.docker.compose.project=socialos, and only reads the protected units' state and
# cgroup files. Settings: GUARD_* in the environment or in .env (guard-measure.sh).
# Exit codes: 0 done (also when it shed load); 1 an action failed (docker did not answer, ...); 2 bad settings.
set -Eeuo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
SOCIALOS_DIR=${SOCIALOS_DIR:-$(dirname "$script_dir")}
STATE_DIR=${GUARD_STATE_DIR:-$SOCIALOS_DIR/.deploy/guard}
DOCKER_TIMEOUT=${GUARD_DOCKER_TIMEOUT:-60}
PROJECT=socialos
# shellcheck source=guard-measure.sh
source "$script_dir/guard-measure.sh"

# Under systemd the journal reads a "<N>" prefix as the priority of the line.
P_WARN="" P_ERR=""
if [ -n "${JOURNAL_STREAM-}" ]; then P_WARN="<4>" P_ERR="<3>"; fi
log() { printf 'socialos-guard: %s\n' "$*"; }
warn() { printf '%ssocialos-guard: %s\n' "$P_WARN" "$*"; }
error() { printf '%ssocialos-guard: %s\n' "$P_ERR" "$*"; }
usage() { sed -n '2,26p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; }
FAILED=0

state_get() { cat "$STATE_DIR/$1" 2>/dev/null || true; }
state_set() { printf '%s\n' "$2" >"$STATE_DIR/$1"; }

# alert NAME MESSAGE / clear_alert NAME: a marker file that stays while the condition lasts.
alert() {
  [ -e "$STATE_DIR/alerts/$1" ] || printf 'since %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >"$STATE_DIR/alerts/$1"
  printf '%s\n' "$2" >"$STATE_DIR/alerts/$1.last"
  warn "ALERT $1: $2"
}
clear_alert() {
  if [ -e "$STATE_DIR/alerts/$1" ]; then log "cleared: $1"; fi
  rm -f "$STATE_DIR/alerts/$1" "$STATE_DIR/alerts/$1.last"
}

# dk ARGS: docker with a time limit, so a hung daemon cannot hang the guard (a graceful stop may take its grace period).
dk() { timeout "$DOCKER_TIMEOUT" docker "$@"; }
# shellcheck source=guard-checks.sh
source "$script_dir/guard-checks.sh"

# stop_service SERVICE: stops the running SocialOS containers of SERVICE (with their own stop grace period) and records
# their ids in $STATE_DIR/stopped, so that a resume starts exactly those and nothing a human stopped.
stop_service() {
  local found id
  if ! found=$(dk ps -q --filter "label=com.docker.compose.project=$PROJECT" \
    --filter "label=com.docker.compose.service=$1" --filter status=running); then
    error "docker did not answer (listing $1): cannot stop it"
    FAILED=1
    return 0
  fi
  for id in $found; do
    if dk stop "$id" >/dev/null; then
      printf '%s %s\n' "$id" "$1" >>"$STATE_DIR/stopped"
      log "stopped $1 ($id)"
    else
      error "docker stop $1 ($id) failed"
      FAILED=1
    fi
  done
}

# handle_thrash: a container that thrashes its page cache (assess_containers) is the cause; shedding others cannot help.
# Raises the alert thrash-<service> naming it, restarts it (an essential service) or stops it (the others, resumed
# like any shed container), at most once per THRASH_COOLDOWN. Sets THRASH_ACTED=true when it did something. The alert
# says what to change: the memory cap is too small for that container (MINIO_MEM_LIMIT and friends in .env).
handle_thrash() {
  local entry svc id pct mbps now at f name acted=false current=" "
  now=${GUARD_NOW:-$(date +%s)}
  THRASH_ACTED=false
  for entry in "${THRASHING[@]+"${THRASHING[@]}"}"; do
    IFS='|' read -r svc id pct mbps <<<"$entry"
    current+="thrash-$svc "
    at=$(state_get "thrash-$svc.at")
    if [ -n "$at" ] && ((at + THRASH_COOLDOWN > now)); then
      alert "thrash-$svc" "$svc is thrashing its page cache (memory at ${pct}% of its cap, ${mbps} MB/s of refaults); already acted at $(date -u -d "@$at" +%H:%M:%SZ 2>/dev/null || echo "$at"), next action after the ${THRASH_COOLDOWN}s cooldown; its memory cap is too small: raise ${svc^^}_MEM_LIMIT in .env"
      continue
    fi
    state_set "thrash-$svc.at" "$now"
    if [[ " $THRASH_ESSENTIAL " == *" $svc "* ]]; then
      if dk restart "$id" >/dev/null; then acted=true; log "restarted $svc ($id): thrashing"; else
        error "docker restart $svc ($id) failed"
        FAILED=1
      fi
      alert "thrash-$svc" "$svc is thrashing its page cache (memory at ${pct}% of its cap, ${mbps} MB/s of refaults): restarted; if it comes back its memory cap is too small: raise ${svc^^}_MEM_LIMIT in .env"
    else
      stop_service "$svc"
      acted=true
      alert "thrash-$svc" "$svc is thrashing its page cache (memory at ${pct}% of its cap, ${mbps} MB/s of refaults): stopped (resumes with the guard's other stops, or socialos-guard.sh --resume); raise ${svc^^}_MEM_LIMIT in .env"
    fi
  done
  for f in "$STATE_DIR"/alerts/thrash-*; do
    case "$f" in *.last | *'*') continue ;; esac
    name=${f##*/}
    case "$current" in *" $name "*) ;; *) clear_alert "$name" ;; esac
  done
  THRASH_ACTED=$acted
}

# pressure_note: what the pressure alert adds about the top IO container, when shedding would not stop it.
pressure_note() {
  local svc=${TOP_IO%% *}
  [ -n "$TOP_IO" ] || return 0
  if [[ " worker $SHED_SERVICES " == *" $svc "* ]]; then
    printf '; top IO: %s MB/s from %s (shed on level 2)' "${TOP_IO#* }" "$svc"
  else
    printf '; top IO: %s MB/s from %s, which shedding does not stop (alert thrash-%s or check its memory cap)' "${TOP_IO#* }" "$svc" "$svc"
  fi
}

shed() { # LEVEL
  local svc services=worker
  if [ "$1" -ge 2 ]; then services=$SHED_SERVICES; fi
  for svc in $services; do stop_service "$svc"; done
}

# resume: starts what the guard stopped. A container that is gone (a deploy recreated it) is reported and skipped.
resume() {
  local id svc
  while read -r id svc; do
    [ -n "$id" ] || continue
    if dk start "$id" >/dev/null 2>&1; then log "started $svc ($id)"; else
      warn "could not start $svc ($id): it may have been replaced by a deploy (docker compose ps)"
    fi
  done < <(state_get stopped)
  rm -f "$STATE_DIR/shed" "$STATE_DIR/stopped" "$STATE_DIR/crit_runs" "$STATE_DIR/calm_runs"
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

# calm_needed: calm runs before an automatic resume. Level 2 backs off: x2 for each level 2 in the last 24 hours.
calm_needed() {
  local n=$RESUME_AFTER episodes i
  if [ "$(state_get shed)" = 2 ]; then
    episodes=$(state_get episodes)
    n=$RESUME_AFTER_CRIT
    for ((i = 1; i < ${episodes:-1}; i++)); do n=$((n * 2)); done
    if ((n > RESUME_AFTER_MAX)); then n=$RESUME_AFTER_MAX; fi
  fi
  printf '%s' "$n"
}

# escalate LEVEL: records the shed level; a new level 2 counts as an episode for the backoff (reset after a quiet day).
escalate() {
  local now at episodes current
  now=${GUARD_NOW:-$(date +%s)}
  current=$(state_get shed)
  if [ "$1" -ge 2 ] && [ "$current" != 2 ]; then
    at=$(state_get episode_at)
    episodes=$(state_get episodes)
    if [ -z "$at" ] || ((at + 86400 < now)); then episodes=0; fi
    state_set episodes $((${episodes:-0} + 1))
    state_set episode_at "$now"
  fi
  if [ "$1" -gt "${current:-0}" ]; then state_set shed "$1"; fi
}

act() {
  local level calm need
  if [ "$LEVEL" -gt 0 ]; then
    state_set calm_runs 0
    if [ "$CONTRIBUTES" = false ]; then
      state_set crit_runs 0
      alert pressure "level $LEVEL: ${REASONS[*]}; $SOCIALOS_USAGE: not a real contributor, so no action$(pressure_note)"
      return 0
    fi
    if [ "$THRASH_ACTED" = true ]; then
      state_set crit_runs 0
      alert pressure "level $LEVEL: ${REASONS[*]}; $SOCIALOS_USAGE$(pressure_note); a thrashing container was restarted or stopped (alert thrash-*): no shedding this run"
      return 0
    fi
    level=$(decide)
    alert pressure "level $level: ${REASONS[*]}; $SOCIALOS_USAGE$(pressure_note)"
    shed "$level"
    escalate "$level"
    alert shed "SocialOS load shed at level $(state_get shed) (1: worker stopped, 2: $SHED_SERVICES stopped); resumes by itself once calm, or now: socialos-guard.sh --resume"
    return 0
  fi
  clear_alert pressure
  [ -n "$(state_get shed)" ] || return 0
  calm=$(state_get calm_runs)
  calm=$((${calm:-0} + 1))
  state_set calm_runs "$calm"
  need=$(calm_needed)
  if [ "$calm" -ge "$need" ]; then
    log "calm for $calm runs: resuming"
    resume
  else
    log "calm for $calm of $need runs before resuming"
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
if [ "$mode" = dry ]; then
  assess
  assess_socialos
  assess_containers
  log "level $LEVEL${REASONS[*]:+: ${REASONS[*]}} (disk ${DISK_PCT}%; $SOCIALOS_USAGE, contributor: $CONTRIBUTES${THRASHING[*]:+; thrashing: ${THRASHING[*]}}); dry run, nothing changed"
  exit 0
fi

mkdir -p "$STATE_DIR/alerts"
exec 8>"$STATE_DIR/lock"
flock -w 120 8 || {
  error "another run holds $STATE_DIR/lock"
  exit 1
}
if [ "$mode" = resume ]; then resume; else
  WRITE_STATE=true
  assess
  assess_socialos
  assess_containers
  handle_thrash
  check_health
  check_slice
  check_budgets
  act
  log "level $LEVEL${REASONS[*]:+: ${REASONS[*]}} (disk ${DISK_PCT}%; $SOCIALOS_USAGE)"
fi
exit "$FAILED"
