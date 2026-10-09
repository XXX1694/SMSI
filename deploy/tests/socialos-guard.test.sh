#!/usr/bin/env bash
# host-proxy/socialos-guard.sh against a fake /proc, fake cgroup files and stubbed docker, systemctl, df, du and curl:
# it sheds Steerpost load only under pressure AND when Steerpost contributes to it, stops gracefully, starts again only what
# it stopped (with a backoff after level 2), alerts without acting otherwise, and never touches the protected services
# or containers of other projects.
# shellcheck source=lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

# setup: a calm host (50% RAM available, no stalls, disk 40%, data 1 GB, slice installed: 200 MB anonymous memory plus 160 MB page cache) and a running
# Steerpost stack.
setup() {
  new_sb
  mkdir -p "$SB/opt" "$SB/proc/pressure" "$SB/docker" "$SB/cg/socialos.slice"
  for svc in postgres redis minio backend worker mcp frontend; do echo running >"$SB/docker/$svc"; done
  mem 50
  psi "$SB/proc/pressure/memory" 0
  for unit in irbisa.service caddy.service; do
    mkdir -p "$SB/cg/system.slice/$unit"
    for res in cpu io memory; do psi "$SB/cg/system.slice/$unit/$res.pressure" 0; done
  done
  slice_mem 200
  echo "usage_usec 1000000" >"$SB/cg/socialos.slice/cpu.stat"
  echo "8:0 rbytes=1000 wbytes=1000 rios=1 wios=1" >"$SB/cg/socialos.slice/io.stat"
  echo 40 >"$SB/disk"
  echo $((1024 * 1024)) >"$SB/du.kb"
  echo 696254464 >"$SB/slice.max"
  echo 200 >"$SB/status"
  cat >"$SB/bin/docker" <<STUB
#!/usr/bin/env bash
echo "docker \$*" >>"$SB/docker.calls"
[ ! -e "$SB/docker.down" ] || exit 1
cmd=\$1
shift
case \$cmd in
  ps)
    svc="" status="" fmt=""
    for arg in "\$@"; do
      case \$arg in label=com.docker.compose.service=*) svc=\${arg##*=} ;; status=*) status=\${arg#status=} ;; '{{.ID}}'*) fmt=1 ;; esac
    done
    for f in "$SB"/docker/*; do
      n=\${f##*/}
      st=\$(cat "\$f")
      if { [ -z "\$svc" ] || [ "\$svc" = "\$n" ]; } && { [ -z "\$status" ] || [ "\$status" = "\$st" ]; }; then if [ -n "\$fmt" ]; then echo "id-\$n \$n"; else echo "id-\$n"; fi; fi
    done
    ;;
  restart) echo running >"$SB/docker/\${1#id-}" ;;
  stop) echo exited >"$SB/docker/\${1#id-}" ;;
  start) [ ! -e "$SB/gone.\${1#id-}" ] || exit 1; echo running >"$SB/docker/\${1#id-}" ;;
  inspect) cat "$SB/cgparent.\${3#id-}" 2>/dev/null || echo socialos.slice ;;
  *) exit 99 ;;
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
slice_mem() { # slice_mem ANON_MB [CACHE_MB]: memory.stat anon/file and memory.current (their sum, as in the kernel)
  printf 'anon %s\nfile %s\nkernel 1000\n' $(($1 * 1048576)) $((${2:-160} * 1048576)) >"$SB/cg/socialos.slice/memory.stat"
  echo $((($1 + ${2:-160}) * 1048576 + 1000)) >"$SB/cg/socialos.slice/memory.current"
}
psi() { printf 'some avg10=0.00 avg60=%s.50 avg300=0.00 total=1\nfull avg10=0.00 avg60=0.00 avg300=0.00 total=1\n' "$2" >"$1"; }
NOW=1000000
guard() { # guard [ARGS]: sets $out and $rc; the clock is $NOW
  rc=0
  out=$(SOCIALOS_DIR="$SB/opt" PROC_DIR="$SB/proc" CGROUP_ROOT="$SB/cg" GUARD_NOW="$NOW" PATH="$SB/bin:$PATH" \
    bash "$REPO_DEPLOY/host-proxy/socialos-guard.sh" "$@" 2>&1) || rc=$?
}
calm_runs() { local i; for ((i = 0; i < $1; i++)); do guard; done; }
states() { local s out=""; for s in postgres redis minio backend worker mcp frontend; do out+="$s=$(cat "$SB/docker/$s") "; done; printf '%s' "${out% }"; }
ALL_RUNNING="postgres=running redis=running minio=running backend=running worker=running mcp=running frontend=running"
LEVEL2="postgres=running redis=running minio=running backend=running worker=exited mcp=exited frontend=exited"
G=opt/.deploy/guard
actions() { grep -cE '^docker (stop|start) ' "$SB/docker.calls" 2>/dev/null || true; }
pressure_last() { cat "$SB/$G/alerts/pressure.last"; }

# 1. a calm host: nothing is done, nothing is alerted
setup
guard
assert_eq "calm: exit" 0 "$rc"
assert_eq "calm: stack untouched" "$ALL_RUNNING" "$(states)"
assert_eq "calm: no docker action" 0 "$(actions)"
assert_no_file "calm: not shed" "$SB/$G/shed"
assert_no_file "calm: no pressure alert" "$SB/$G/alerts/pressure"
assert_has "calm: logs level 0" "$out" "level 0"

# 2. memory pressure while Steerpost uses 450 MB anon: the worker is stopped gracefully (no pause), and started after 3 calm runs
setup
mem 12
slice_mem 450
guard
assert_eq "pressure: exit" 0 "$rc"
assert_eq "pressure: worker stopped, nothing else" "${ALL_RUNNING/worker=running/worker=exited}" "$(states)"
assert_has "pressure: a docker stop with the container's own grace period" "$(cat "$SB/docker.calls")" "docker stop id-worker"
assert_lacks "pressure: never paused" "$(cat "$SB/docker.calls")" "pause"
assert_eq "pressure: shed level 1" 1 "$(cat "$SB/$G/shed")"
assert_has "pressure: reason" "$(pressure_last)" "available memory 12% < 15%"
assert_has "pressure: usage" "$(pressure_last)" "Steerpost uses 450 MB anon"
mem 50
calm_runs 2
assert_eq "two calm runs: still stopped" exited "$(cat "$SB/docker/worker")"
guard
assert_eq "third calm run: started again" "$ALL_RUNNING" "$(states)"
assert_no_file "resumed: shed marker gone" "$SB/$G/shed"
assert_no_file "resumed: alerts cleared" "$SB/$G/alerts/pressure"

# 3. pressure that Steerpost does not cause (200 MB anon, no CPU, no IO): alert, but nothing is stopped
setup
mem 5
guard
guard
assert_eq "not a contributor: stack untouched" "$ALL_RUNNING" "$(states)"
assert_eq "not a contributor: no docker action" 0 "$(actions)"
assert_has "not a contributor: alert says why" "$(pressure_last)" "not a real contributor, so no action"
assert_no_file "not a contributor: not shed" "$SB/$G/shed"

# 3b. a lot of page cache but little anonymous memory: memory.current is 700 MB, yet Steerpost is not a contributor
setup
mem 5
slice_mem 150 550
guard
guard
assert_eq "high cache, low anon: stack untouched" "$ALL_RUNNING" "$(states)"
assert_eq "high cache, low anon: no docker action" 0 "$(actions)"
assert_has "high cache, low anon: usage counts anon only" "$(pressure_last)" "Steerpost uses 150 MB anon"
assert_no_file "high cache, low anon: not shed" "$SB/$G/shed"

# 3c. no memory.stat: falls back to memory.current minus "file"... and to memory.current alone without both
setup
mem 12
slice_mem 100 500
rm "$SB/cg/socialos.slice/memory.stat"
guard
assert_eq "no memory.stat: memory.current used as the last resort, worker stopped" exited "$(cat "$SB/docker/worker")"

# 3d. high anon and pressure: the worker is stopped (also with little cache)
setup
mem 12
slice_mem 350 0
guard
assert_eq "high anon, pressure: worker stopped" exited "$(cat "$SB/docker/worker")"
assert_has "high anon, pressure: usage" "$(pressure_last)" "Steerpost uses 350 MB anon"

# 4. Steerpost contributes through CPU: the rate needs two runs (60% of one CPU over 120 s)
setup
psi "$SB/cg/system.slice/irbisa.service/cpu.pressure" 25
guard
assert_eq "cpu, first run: no rate yet, no action" 0 "$(actions)"
echo "usage_usec $((1000000 + 72000000))" >"$SB/cg/socialos.slice/cpu.stat"
NOW=$((NOW + 120)) guard
assert_eq "cpu, second run: worker stopped" exited "$(cat "$SB/docker/worker")"
assert_has "cpu: usage shown" "$(pressure_last)" "60% CPU"
assert_has "cpu: unit reason" "$(pressure_last)" "irbisa.service cpu stall 25% >= 20%"
# ... and through IO (20 MB/s)
setup
psi "$SB/cg/system.slice/irbisa.service/io.pressure" 30
guard
echo "8:0 rbytes=$((1000 + 2400 * 1048576)) wbytes=1000" >"$SB/cg/socialos.slice/io.stat"
NOW=$((NOW + 120)) guard
assert_eq "io: worker stopped" exited "$(cat "$SB/docker/worker")"
assert_has "io: usage shown" "$(pressure_last)" "20 MB/s IO"

# 5. critical twice: worker, mcp and frontend stop; data services keep running; resume after 5 calm runs, 10 the next time
setup
mem 5
slice_mem 450
guard
assert_eq "critical once: only the worker" "${ALL_RUNNING/worker=running/worker=exited}" "$(states)"
guard
assert_eq "critical twice: shed services stopped" "$LEVEL2" "$(states)"
assert_eq "critical: shed level 2" 2 "$(cat "$SB/$G/shed")"
mem 50
calm_runs 4
assert_eq "level 2, 4 calm runs: still stopped" "$LEVEL2" "$(states)"
assert_has "level 2: says how long" "$out" "calm for 4 of 5 runs"
guard
assert_eq "level 2, 5 calm runs: started again" "$ALL_RUNNING" "$(states)"
mem 5
guard
guard
assert_eq "second level 2: stopped again" "$LEVEL2" "$(states)"
mem 50
calm_runs 9
assert_eq "second level 2 the same day: 9 calm runs are not enough" "$LEVEL2" "$(states)"
guard
assert_eq "second level 2: started after 10" "$ALL_RUNNING" "$(states)"
mem 5
NOW=$((NOW + 2 * 86400)) guard
NOW=$((NOW + 2 * 86400)) guard
mem 50
for _ in 1 2 3 4 5; do NOW=$((NOW + 2 * 86400)) guard; done
assert_eq "a level 2 after a quiet day: back to 5 runs" "$ALL_RUNNING" "$(states)"

# 6. only what the guard stopped is started: a container a human stopped stays stopped; a replaced one is reported
setup
echo exited >"$SB/docker/mcp"
mem 5
slice_mem 450
guard
guard
touch "$SB/gone.frontend"
guard --resume
assert_eq "--resume: exit" 0 "$rc"
assert_eq "--resume: the human-stopped mcp stays stopped" exited "$(cat "$SB/docker/mcp")"
assert_eq "--resume: the worker runs" running "$(cat "$SB/docker/worker")"
assert_has "--resume: a replaced container is reported" "$out" "could not start frontend"
assert_no_file "--resume: shed marker gone" "$SB/$G/shed"

# 7. a protected service is down or a health URL fails on a calm host: alert only, nothing is shed or touched
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
setup
mem 50 90
slice_mem 450
guard
assert_has "swap almost full: reason" "$(pressure_last)" "swap in use 90% >= 80%"

# 8. disk: alerts only (stopping containers frees no space); the data budget is measured at most once an hour
setup
echo 95 >"$SB/disk"
echo $((16 * 1024 * 1024)) >"$SB/du.kb"
slice_mem 450
guard
guard
assert_file "disk 95%: alert" "$SB/$G/alerts/disk"
assert_eq "disk 95%: nothing stopped" "$ALL_RUNNING" "$(states)"
assert_has "data budget: alert" "$(cat "$SB/$G/alerts/data-budget.last")" "16 GB >= 15 GB"
assert_eq "data budget: du once an hour" 1 "$(wc -l <"$SB/du.calls" | tr -d ' ')"

# 9. the slice: missing limits or a container outside it is an alert; without the slice's files the guard still protects
setup
echo infinity >"$SB/slice.max"
guard
assert_has "slice without limits: alert" "$(cat "$SB/$G/alerts/slice.last")" "no memory limit"
setup
echo system.slice >"$SB/cgparent.frontend"
guard
assert_has "container outside the slice: alert" "$(cat "$SB/$G/alerts/slice.last")" "1 Steerpost container(s) run outside"
rm "$SB/cgparent.frontend"
guard
assert_no_file "all inside: alert cleared" "$SB/$G/alerts/slice"
setup
rm -r "$SB/cg/socialos.slice"
mem 12
guard
assert_eq "no slice files: assumed a contributor, worker stopped" exited "$(cat "$SB/docker/worker")"

# 10. docker does not answer while Steerpost adds to the pressure: exit 1 and a clear error
setup
mem 5
slice_mem 450
touch "$SB/docker.down"
guard
assert_eq "docker down: exit" 1 "$rc"
assert_has "docker down: explained" "$out" "docker did not answer"

# 11. dry run: measures and says, changes nothing
setup
mem 5
guard --dry-run
assert_eq "dry run: exit" 0 "$rc"
assert_has "dry run: level shown" "$out" "level 2: available memory 5% < 8%"
assert_has "dry run: contribution shown" "$out" "contributor: false"
assert_eq "dry run: stack untouched" "$ALL_RUNNING" "$(states)"
assert_no_file "dry run: no state" "$SB/$G"

# 12. settings: from .env, environment wins, bad values refused
setup
mem 40
slice_mem 450
echo "GUARD_MEM_AVAIL_WARN=50" >"$SB/opt/.env"
guard
assert_eq "threshold from .env" 1 "$(cat "$SB/$G/shed")"
setup
GUARD_MEM_AVAIL_WARN=abc guard
assert_eq "bad setting: exit" 2 "$rc"
assert_has "bad setting: explained" "$out" "GUARD_MEM_AVAIL_WARN must be a whole number"

# 13. only Steerpost containers: every docker ps is filtered by the compose project; --status
setup
mem 5
slice_mem 450
guard
guard
assert_eq "every docker ps is scoped to the socialos project" 0 \
  "$(grep '^docker ps' "$SB/docker.calls" | grep -vc 'label=com.docker.compose.project=socialos' || true)"
guard --status
assert_has "--status: level" "$out" "shed level: 2"
assert_has "--status: alerts" "$out" "pressure: since"

# 14. a container that thrashes its page cache (MinIO at its cap, refaults 56 MB/s): named in an alert; on a stressed host,
# after two consecutive readings and outside a deploy it is restarted once per episode; nothing is shed that run
ctr() { # ctr SERVICE MEM_PCT REFAULT_PAGES [IO_BYTES]: the container's cgroup files (cap 192 MiB)
  local d="$SB/cg/socialos.slice/docker-id-$1.scope"
  mkdir -p "$d"
  echo $((192 * 1048576)) >"$d/memory.max"
  echo $((192 * 1048576 * $2 / 100)) >"$d/memory.current"
  printf 'anon 1000\nworkingset_refault_file %s\n' "$3" >"$d/memory.stat"
  echo "8:0 rbytes=${4:-0} wbytes=0" >"$d/io.stat"
}
STEP=0
tick() { # tick SERVICE: one guard run 120 s later in which SERVICE thrashes harder (56 MB/s) and Steerpost reads 57 MB/s
  STEP=$((STEP + 1))
  NOW=$((NOW + 120))
  ctr "$1" 100 $((56 * 256 * 120 * STEP)) $((56 * 120 * 1048576 * STEP))
  echo "8:0 rbytes=$((1000 + 6840 * 1048576 * STEP)) wbytes=1000" >"$SB/cg/socialos.slice/io.stat"
  guard
}
thrash_setup() { # IO pressure caused by Steerpost; the first run only records counters
  setup
  STEP=0
  psi "$SB/cg/system.slice/irbisa.service/io.pressure" 60
  ctr "$1" 100 0 0
  guard
  assert_eq "thrash: first run has no rate yet, nothing done" 0 "$(actions)"
}
restarts() { grep -c '^docker restart' "$SB/docker.calls" || true; }
thrash_setup minio
tick minio
assert_eq "thrash: one reading is not enough" 0 "$(restarts)"
assert_has "thrash: first reading alerts" "$(cat "$SB/$G/alerts/thrash-minio.last")" "waiting for a second reading"
tick minio
assert_eq "thrash: exit" 0 "$rc"
assert_has "thrash: minio restarted" "$(cat "$SB/docker.calls")" "docker restart id-minio"
assert_eq "thrash: the restarted minio runs, mcp and frontend are not shed" "running running running" "$(cat "$SB/docker/minio" "$SB/docker/mcp" "$SB/docker/frontend" | paste -sd' ')"
assert_has "thrash: alert names the container" "$(cat "$SB/$G/alerts/thrash-minio.last")" "minio is thrashing its page cache (memory at 100% of its cap, 56 MB/s of refaults)"
assert_has "thrash: alert says what to raise" "$(cat "$SB/$G/alerts/thrash-minio.last")" "MINIO_MEM_LIMIT"
assert_has "thrash: pressure says shedding does not help" "$(pressure_last)" "no shedding this run"
assert_has "thrash: pressure names the IO container" "$(pressure_last)" "top IO: 56 MB/s from minio, which shedding does not stop"
# it comes back: no second restart, not even after the cooldown; the usual shedding applies
tick minio
tick minio
tick minio
assert_eq "repeat thrash: still one restart" 1 "$(restarts)"
assert_has "repeat thrash: alert only" "$(cat "$SB/$G/alerts/thrash-minio.last")" "already acted"
assert_eq "repeat thrash: the usual shedding applies" exited "$(cat "$SB/docker/worker")"
# it stops thrashing: the alert clears, and after the cooldown a new episode may be acted on again
ctr minio 50 0 0
NOW=$((NOW + 7200))
guard
assert_no_file "thrash: recovered, alert cleared" "$SB/$G/alerts/thrash-minio"
assert_no_file "thrash: episode ended after the cooldown" "$SB/$G/thrash-minio.at"

# 14b. a non-essential container is stopped instead; it starts again by itself after the calm runs
thrash_setup frontend
tick frontend
tick frontend
assert_eq "thrash, frontend: stopped, not restarted" exited "$(cat "$SB/docker/frontend")"
assert_eq "thrash, frontend: no restart" 0 "$(restarts)"
assert_has "thrash, frontend: alert" "$(cat "$SB/$G/alerts/thrash-frontend.last")" "starts again after 3 calm runs"
assert_eq "thrash, frontend: shed level 1 recorded" 1 "$(cat "$SB/$G/shed")"
psi "$SB/cg/system.slice/irbisa.service/io.pressure" 0
ctr frontend 30 0 0
calm_runs 3
assert_eq "thrash, frontend: started again by the normal resume" running "$(cat "$SB/docker/frontend")"

# 14c. a calm host: the same thrash is an alert only; and full memory without refaults is left alone
thrash_setup postgres
psi "$SB/cg/system.slice/irbisa.service/io.pressure" 0
tick postgres
tick postgres
tick postgres
assert_eq "calm host: no restart" 0 "$(restarts)"
assert_eq "calm host: no docker action" 0 "$(actions)"
assert_has "calm host: alert only" "$(cat "$SB/$G/alerts/thrash-postgres.last")" "host is calm"
setup
ctr minio 100 1000 0
ctr backend 50 1000 0
guard
ctr minio 100 1000 0
ctr backend 50 $((500 * 256 * 120)) 0
NOW=$((NOW + 120))
guard
assert_eq "normal: no docker action" 0 "$(actions)"
assert_eq "normal: no restart" 0 "$(restarts)"
assert_no_file "normal: no thrash alert" "$SB/$G/alerts/thrash-minio"
assert_no_file "normal: no thrash alert (backend)" "$SB/$G/alerts/thrash-backend"

# 14d. a deploy holds its lock: alert only; once the lock is free the guard acts
thrash_setup minio
mkdir -p "$SB/opt/.deploy"
: >"$SB/opt/.deploy/lock"
( exec 9>"$SB/opt/.deploy/lock"; flock -n 9 && exec sleep 30 ) &
holder=$!
sleep 1
tick minio
tick minio
assert_eq "deploy running: no restart" 0 "$(restarts)"
assert_has "deploy running: alert says so" "$(cat "$SB/$G/alerts/thrash-minio.last")" "a deploy is running"
kill "$holder" 2>/dev/null || true
wait "$holder" 2>/dev/null || true
tick minio
assert_eq "deploy finished: restarted" 1 "$(restarts)"

# 14e. dry run names the thrashing container and changes nothing
thrash_setup minio
ctr minio 100 $((56 * 256 * 120)) 0
NOW=$((NOW + 120))
guard --dry-run
assert_has "dry run: thrashing shown" "$out" "thrashing: minio|id-minio|100|56"
assert_lacks "dry run: no restart" "$(cat "$SB/docker.calls")" "restart"

finish
