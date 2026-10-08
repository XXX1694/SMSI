#!/usr/bin/env bash
# host-proxy/socialos-guard.sh against a fake /proc, fake cgroup pressure files and stubbed docker, systemctl, df, du and
# curl: it sheds SocialOS load only under pressure, in the right order, alerts without acting otherwise, and never touches
# the protected services or containers of other projects.
# shellcheck source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

# setup: a calm host (50% RAM available, no stalls, disk 40%, data 1 GB, slice installed) and a running SocialOS stack.
setup() {
  new_sb
  mkdir -p "$SB/opt" "$SB/proc/pressure" "$SB/docker"
  for svc in postgres redis minio backend worker mcp frontend; do echo running >"$SB/docker/$svc"; done
  mem 50
  psi "$SB/proc/pressure/memory" 0
  for unit in irbisa.service caddy.service; do
    mkdir -p "$SB/cg/system.slice/$unit"
    for res in cpu io memory; do psi "$SB/cg/system.slice/$unit/$res.pressure" 0; done
  done
  echo 40 >"$SB/disk"
  echo $((1024 * 1024)) >"$SB/du.kb"
  echo 629145600 >"$SB/slice.max"
  echo 200 >"$SB/status"
  cat >"$SB/bin/docker" <<STUB
#!/usr/bin/env bash
echo "docker \$*" >>"$SB/docker.calls"
[ ! -e "$SB/docker.down" ] || exit 1
cmd=\$1
shift
case \$cmd in
  ps)
    svc="" status=""
    for arg in "\$@"; do
      case \$arg in label=com.docker.compose.service=*) svc=\${arg##*=} ;; status=*) status=\${arg#status=} ;; esac
    done
    for f in "$SB"/docker/*; do
      n=\${f##*/}
      st=\$(cat "\$f")
      if { [ -z "\$svc" ] || [ "\$svc" = "\$n" ]; } && { [ -z "\$status" ] || [ "\$status" = "\$st" ]; }; then echo "id-\$n"; fi
    done
    ;;
  pause) echo paused >"$SB/docker/\${1#id-}" ;;
  unpause | start) echo running >"$SB/docker/\${1#id-}" ;;
  stop) echo exited >"$SB/docker/\${1#id-}" ;;
  inspect) cat "$SB/cgparent.\${3#id-}" 2>/dev/null || echo socialos.slice ;;
esac
STUB
  cat >"$SB/bin/systemctl" <<STUB
#!/usr/bin/env bash
echo "systemctl \$*" >>"$SB/systemctl.calls"
case \$1 in
  is-active) [ ! -e "$SB/inactive.\$3" ] ;;
  show) cat "$SB/slice.max" ;;
  *) exit 99 ;;
esac
STUB
  cat >"$SB/bin/df" <<STUB
#!/usr/bin/env bash
printf 'Filesystem 1024-blocks Used Available Capacity Mounted on\n/dev/sda2 40000000 1 1 %s%% /\n' "\$(cat "$SB/disk")"
STUB
  cat >"$SB/bin/du" <<STUB
#!/usr/bin/env bash
echo du >>"$SB/du.calls"
printf '%s\t/var/lib/docker\n' "\$(cat "$SB/du.kb")"
STUB
  printf '#!/usr/bin/env bash\ncat "%s/status"\n' "$SB" >"$SB/bin/curl"
  chmod +x "$SB"/bin/*
}
mem() { # mem AVAILABLE_PCT [SWAP_USED_PCT]
  printf 'MemTotal: 2000000 kB\nMemAvailable: %s kB\nSwapTotal: 2000000 kB\nSwapFree: %s kB\n' \
    $((20000 * $1)) $((20000 * (100 - ${2:-0}))) >"$SB/proc/meminfo"
}
psi() { printf 'some avg10=0.00 avg60=%s.50 avg300=0.00 total=1\nfull avg10=0.00 avg60=0.00 avg300=0.00 total=1\n' "$2" >"$1"; }
guard() { # guard [ARGS]: sets $out and $rc
  rc=0
  out=$(SOCIALOS_DIR="$SB/opt" PROC_DIR="$SB/proc" CGROUP_ROOT="$SB/cg" PATH="$SB/bin:$PATH" \
    bash "$REPO_DEPLOY/host-proxy/socialos-guard.sh" "$@" 2>&1) || rc=$?
}
states() { local s out=""; for s in postgres redis minio backend worker mcp frontend; do out+="$s=$(cat "$SB/docker/$s") "; done; printf '%s' "${out% }"; }
ALL_RUNNING="postgres=running redis=running minio=running backend=running worker=running mcp=running frontend=running"
G=opt/.deploy/guard
actions() { grep -cE '^docker (pause|unpause|stop|start) ' "$SB/docker.calls" 2>/dev/null || true; }

# 1. a calm host: nothing is done, nothing is alerted
setup
guard
assert_eq "calm: exit" 0 "$rc"
assert_eq "calm: stack untouched" "$ALL_RUNNING" "$(states)"
assert_eq "calm: no docker action" 0 "$(actions)"
assert_no_file "calm: not shed" "$SB/$G/shed"
assert_no_file "calm: no pressure alert" "$SB/$G/alerts/pressure"
assert_has "calm: logs level 0" "$out" "level 0"

# 2. memory pressure: only the worker is paused; three calm runs later it is resumed by itself
setup
mem 12
guard
assert_eq "pressure: exit" 0 "$rc"
assert_eq "pressure: worker paused, nothing else" "${ALL_RUNNING/worker=running/worker=paused}" "$(states)"
assert_eq "pressure: shed level 1" 1 "$(cat "$SB/$G/shed")"
assert_has "pressure: reason" "$(cat "$SB/$G/alerts/pressure.last")" "available memory 12% < 15%"
assert_file "pressure: shed alert" "$SB/$G/alerts/shed"
mem 50
guard
guard
assert_eq "two calm runs: still paused" paused "$(cat "$SB/docker/worker")"
guard
assert_eq "third calm run: resumed" "$ALL_RUNNING" "$(states)"
assert_no_file "resumed: shed marker gone" "$SB/$G/shed"
assert_no_file "resumed: alerts cleared" "$SB/$G/alerts/pressure"

# 3. critical: the first reading only pauses, the second stops worker, mcp and frontend; data services keep running
setup
mem 5
guard
assert_eq "critical once: only paused" "${ALL_RUNNING/worker=running/worker=paused}" "$(states)"
guard
assert_eq "critical twice: shed services stopped" \
  "postgres=running redis=running minio=running backend=running worker=exited mcp=exited frontend=exited" "$(states)"
assert_eq "critical: shed level 2" 2 "$(cat "$SB/$G/shed")"
assert_has "critical: the paused worker was unpaused before the stop" "$(cat "$SB/docker.calls")" "docker unpause id-worker"
mem 50
for _ in 1 2 3 4; do guard; done
assert_eq "level 2 is not undone by itself" exited "$(cat "$SB/docker/frontend")"
assert_has "level 2: says how to resume" "$out" "socialos-guard.sh --resume"
guard --resume
assert_eq "--resume: exit" 0 "$rc"
assert_eq "--resume: everything runs again" "$ALL_RUNNING" "$(states)"
assert_no_file "--resume: shed marker gone" "$SB/$G/shed"

# 4. a protected service waits for IO / CPU (its own PSI files): critical / pressure
setup
psi "$SB/cg/system.slice/irbisa.service/io.pressure" 60
psi "$SB/cg/system.slice/caddy.service/cpu.pressure" 25
guard
assert_has "unit PSI: io reason" "$(cat "$SB/$G/alerts/pressure.last")" "irbisa.service io stall 60% >= 50%"
assert_has "unit PSI: cpu reason" "$(cat "$SB/$G/alerts/pressure.last")" "caddy.service cpu stall 25% >= 20%"
setup
psi "$SB/proc/pressure/memory" 12
guard
assert_has "host memory stall: shed" "$(cat "$SB/$G/shed")" 1
setup
mem 50 90
guard
assert_has "swap almost full: reason" "$(cat "$SB/$G/alerts/pressure.last")" "swap in use 90% >= 80%"

# 5. a protected service is down or a health URL fails, but the host is calm: alert only, nothing is shed or touched
setup
touch "$SB/inactive.irbisa.service"
echo 502 >"$SB/status"
GUARD_HEALTH_URLS="http://127.0.0.1:3001/health" guard
assert_eq "irbisa down: exit" 0 "$rc"
assert_file "irbisa down: alert" "$SB/$G/alerts/irbisa.service-inactive"
assert_file "health URL: alert" "$SB/$G/alerts/url-http___127.0.0.1_3001_health"
assert_eq "irbisa down: no docker action" 0 "$(actions)"
assert_lacks "never starts, stops or restarts a unit" "$(cat "$SB/systemctl.calls")" "start"
assert_lacks "never stops a unit" "$(cat "$SB/systemctl.calls")" "stop"
rm "$SB/inactive.irbisa.service"
guard
assert_no_file "irbisa back: alert cleared" "$SB/$G/alerts/irbisa.service-inactive"

# 6. disk: an alert at 80%, shedding at 92% (twice in a row); the data budget is measured at most once an hour
setup
echo 85 >"$SB/disk"
echo $((16 * 1024 * 1024)) >"$SB/du.kb"
guard
assert_file "disk 85%: alert" "$SB/$G/alerts/disk"
assert_no_file "disk 85%: not shed" "$SB/$G/shed"
assert_has "data budget: alert" "$(cat "$SB/$G/alerts/data-budget.last")" "16 GB >= 15 GB"
guard
assert_eq "data budget: du once an hour" 1 "$(wc -l <"$SB/du.calls" | tr -d ' ')"
echo 95 >"$SB/disk"
guard
guard
assert_eq "disk 95%: shed level 2" 2 "$(cat "$SB/$G/shed")"

# 7. the slice: missing limits, or a container outside it, is an alert
setup
echo infinity >"$SB/slice.max"
guard
assert_has "slice without limits: alert" "$(cat "$SB/$G/alerts/slice.last")" "no memory limit"
setup
echo system.slice >"$SB/cgparent.frontend"
guard
assert_has "container outside the slice: alert" "$(cat "$SB/$G/alerts/slice.last")" "1 SocialOS container(s) run outside"
rm "$SB/cgparent.frontend"
guard
assert_no_file "all inside: alert cleared" "$SB/$G/alerts/slice"

# 8. docker does not answer while the host is under pressure: exit 1 and a clear error, no false comfort
setup
mem 5
touch "$SB/docker.down"
guard
assert_eq "docker down: exit" 1 "$rc"
assert_has "docker down: explained" "$out" "docker did not answer"

# 9. dry run: measures and says, changes nothing
setup
mem 5
guard --dry-run
assert_eq "dry run: exit" 0 "$rc"
assert_has "dry run: level shown" "$out" "level 2: available memory 5% < 8%"
assert_eq "dry run: stack untouched" "$ALL_RUNNING" "$(states)"
assert_no_file "dry run: no state" "$SB/$G"

# 10. settings: from .env, environment wins, bad values refused
setup
mem 40
echo "GUARD_MEM_AVAIL_WARN=50" >"$SB/opt/.env"
guard
assert_eq "threshold from .env" 1 "$(cat "$SB/$G/shed")"
setup
GUARD_MEM_AVAIL_WARN=abc guard
assert_eq "bad setting: exit" 2 "$rc"
assert_has "bad setting: explained" "$out" "GUARD_MEM_AVAIL_WARN must be a whole number"

# 11. only SocialOS containers: every docker ps is filtered by the compose project
setup
mem 5
guard
guard
assert_eq "every docker ps is scoped to the socialos project" 0 \
  "$(grep '^docker ps' "$SB/docker.calls" | grep -vc 'label=com.docker.compose.project=socialos' || true)"

# 12. --status
guard --status
assert_has "--status: level" "$out" "shed level: 2"
assert_has "--status: alerts" "$out" "pressure: since"

finish
