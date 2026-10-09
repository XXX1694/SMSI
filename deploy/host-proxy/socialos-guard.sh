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
# GUARD_THRASH_REFAULT_MBPS) is the cause no shedding can fix: the guard raises the alert thrash-<service> and, when two
# consecutive runs saw it, the host is under pressure and no deploy runs, restarts it (GUARD_THRASH_ESSENTIAL: postgres
# redis minio backend) or stops it (the others) once per episode (GUARD_THRASH_COOLDOWN), and skips shedding that run. The pressure alert also names an IO-heavy container that shedding would not stop.
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

# deploying: true while deploy.sh holds its lock (.deploy/lock; the guard never creates that file).
deploying() {
  local lock=$SOCIALOS_DIR/.deploy/lock
  [ -e "$lock" ] || return 1
  ! (flock -n 9) 9<"$lock" 2>/dev/null
}

# handle_thrash: a container that thrashes its page cache (assess_containers) is the cause; shedding others cannot help.
# Always raises the alert thrash-<service> (naming the cap to raise). It acts only when (1) two consecutive runs saw the
# container thrash, (2) the host is under pressure (LEVEL > 0: on a calm host a restart cures nothing), (3) no deploy
# runs, and (4) it has not acted on this service in this episode (an episode ends at the first run without thrash, and
# not before THRASH_COOLDOWN): a restart cannot fix a cap that is too small, so a recurrence is an alert only. The action
# restarts an essential service, or stops another one (shed level 1: it starts again after the usual calm runs).
# Sets THRASH_ACTED=true when it did something.
handle_thrash() {
  local entry svc id pct mbps now at streak f name acted=false current=" " why base
  now=${GUARD_NOW:-$(date +%s)}
  THRASH_ACTED=false
  for entry in "${THRASHING[@]+"${THRASHING[@]}"}"; do
    IFS='|' read -r svc id pct mbps <<<"$entry"
    current+="$svc "
    streak=$(state_get "thrash-$svc.streak")
    streak=$((${streak:-0} + 1))
    state_set "thrash-$svc.streak" "$streak"
    base="$svc is thrashing its page cache (memory at ${pct}% of its cap, ${mbps} MB/s of refaults); its memory cap is too small: raise ${svc^^}_MEM_LIMIT in .env"
    at=$(state_get "thrash-$svc.at")
    why=""
    if [ -n "$at" ]; then why="already acted at $at (a restart cannot fix a small cap)"; fi
    if [ -z "$why" ] && ((streak < 2)); then why="seen once, waiting for a second reading"; fi
    if [ -z "$why" ] && ((LEVEL == 0)); then why="host is calm: nothing to relieve, no action"; fi
    if [ -z "$why" ] && deploying; then why="a deploy is running: no action"; fi
    if [ -n "$why" ]; then
      alert "thrash-$svc" "$base; $why"
      continue
    fi
    state_set "thrash-$svc.at" "$now"
    if [[ " $THRASH_ESSENTIAL " == *" $svc "* ]]; then
      if dk restart "$id" >/dev/null; then acted=true; log "restarted $svc ($id): thrashing"; else
        error "docker restart $svc ($id) failed"
        FAILED=1
      fi
      alert "thrash-$svc" "$base; restarted once, no second restart for this episode"
    else
      stop_service "$svc"
      escalate 1
      acted=true
      alert "thrash-$svc" "$base; stopped, starts again after $RESUME_AFTER calm runs (or socialos-guard.sh --resume)"
    fi
  done
  for f in "$STATE_DIR"/thrash-*.streak "$STATE_DIR"/thrash-*.at; do
    case "$f" in *'*'*) continue ;; esac
    name=${f##*/}
    name=${name#thrash-}
    svc=${name%.*}
    case "$current" in *" $svc "*) continue ;; esac
    case "$f" in
      *.streak) rm -f "$f" ;;
      *.at) at=$(cat "$f"); if ((at + THRASH_COOLDOWN <= now)); then rm -f "$f"; fi ;;
    esac
  done
  for f in "$STATE_DIR"/alerts/thrash-*; do
    case "$f" in *.last | *'*') continue ;; esac
    name=${f##*/}
    case "$current" in *" ${name#thrash-} "*) ;; *) clear_alert "$name" ;; esac
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
