# Applying the host guardrails on the shared production host

Runbook for putting SocialOS under the "at most half of the host" limits on the shared VPS (2 vCPU, 1.97 GB RAM, 2 GB swap,
40 GB disk) next to Irbisa (`irbisa.service` on :3001 behind the systemd `caddy`). Design and reasons: `deploy/README.md`,
section 16, and `docs/DECISIONS.md` D-012. Run the steps **in order, one at a time, as root** (`sudo -i`), and stop at the first
check that does not match. Every step says what it changes, its risk, how to verify it and how to roll it back. No step restarts
Docker, containerd, Caddy or Irbisa; only step 3 restarts the SocialOS containers.

Measured on 2026-10-09 (before): SocialOS containers about 360 MB with page cache, about 115 MB of it process memory
(peaks with cache: frontend 149, postgres 92, backend 92, worker 91, minio 46, mcp 45, redis 15 MB); dockerd with
docker-proxy 145 MB (peak 173 MB); containerd with 7 shims 310 MB, 60 MB of it process memory (peak 963 MB, page cache,
during the first image pull); Irbisa 75 MB (peak 178 MB); Caddy 50 MB; disk 8.6 GB of 40 GB used (Docker plus containerd 2.5 GB, journal 373 MB, `/var/backups/socialos` 64 KB). Docker
29.8, cgroup v2, cgroup driver `systemd`, live-restore on: **no `daemon.json` change and no Docker restart are needed.**

## Irbisa check (run before and after every step)

```bash
irbisa_check() {
  systemctl is-active irbisa caddy                                            # expect: active, active
  curl -s -o /dev/null -w 'irbisa.com %{http_code}\n' --max-time 10 https://irbisa.com/     # expect: 200
  curl -s -o /dev/null -w 'local :3001 %{http_code}\n' --max-time 5 http://127.0.0.1:3001/  # expect: below 500 (404 today)
  systemctl show irbisa caddy -p NRestarts --value | paste -sd' '               # expect: unchanged (0 0 today)
  free -m | awk 'NR==2 { print "MemAvailable MB:", $7 }'
  grep some /proc/pressure/memory /sys/fs/cgroup/system.slice/irbisa.service/{cpu,io}.pressure
}
irbisa_check
```

If an "after" check differs from its "before", roll back that step first, then investigate.

## Step 0: preconditions and backup (read-only for the host; risk: none)

```bash
cd /opt/socialos
systemctl show irbisa caddy docker containerd -p NeedDaemonReload --value   # must be "no" four times
docker info -f '{{.CgroupDriver}} {{.CgroupVersion}} {{.LiveRestoreEnabled}}' # must be: systemd 2 true
df -h / ; free -m                                                            # disk below 80%, MemAvailable above 600 MB
tar -C /opt -czf "/root/socialos-files-$(date -u +%Y%m%dT%H%M%SZ).tgz" socialos   # contains .env: stays in /root (mode 700)
```

`NeedDaemonReload=yes` means someone changed one of these unit files without reloading: a `systemctl daemon-reload` below
would activate that change too. Stop and ask the owner of that unit.

Put the files of the branch (or the release that contains it) next to the live ones, without using them yet. From a checkout,
on your machine:

```bash
git archive feat/host-guardrails deploy | ssh irbisa 'rm -rf ~/guardrails && mkdir ~/guardrails && tar -x -C ~/guardrails'
```

On the server, `NEW=/home/<ssh user>/guardrails/deploy` below. Nothing reads that folder.

## Step 1: install the slice unit (risk: none; nothing runs in it yet)

```bash
install -o root -g root -m 644 "$NEW/systemd/socialos.slice" /etc/systemd/system/socialos.slice
systemctl daemon-reload
systemctl show socialos.slice -p CPUQuotaPerSecUSec,MemoryHigh,MemoryMax,MemorySwapMax,TasksMax,IOReadBandwidthMax
```

Verify: `CPUQuotaPerSecUSec=1s`, `MemoryHigh=645922816`, `MemoryMax=696254464`, `MemorySwapMax=0`, `TasksMax=512`,
`IOReadBandwidthMax=/var/lib/docker 62914560` (no write cap on purpose, README section 16). Irbisa check.
Rollback: `rm /etc/systemd/system/socialos.slice && systemctl daemon-reload`.

## Step 2: install the new files in /opt/socialos (risk: none; running containers are not changed)

The autoupdate timer could run `deploy.sh` (and so `docker compose up`) with the new compose file before step 3. Pause it
for the duration:

```bash
systemctl stop socialos-autoupdate.timer
install -o root -g root -m 644 "$NEW/docker-compose.host-proxy.yml" "$NEW/docker-compose.prod.yml" "$NEW/.env.prod.example" "$NEW/README.md" /opt/socialos/
install -o root -g root -m 755 "$NEW/autoupdate.sh" /opt/socialos/
install -o root -g root -m 755 "$NEW"/host-proxy/*.sh /opt/socialos/host-proxy/
install -o root -g root -m 644 "$NEW"/host-proxy/apply-guardrails.md /opt/socialos/host-proxy/
install -d -o root -g root -m 755 /opt/socialos/systemd/docker.service.d /opt/socialos/systemd/containerd.service.d
install -o root -g root -m 644 "$NEW"/systemd/socialos* /opt/socialos/systemd/
install -o root -g root -m 644 "$NEW/systemd/docker.service.d/socialos.conf" /opt/socialos/systemd/docker.service.d/
install -o root -g root -m 644 "$NEW/systemd/containerd.service.d/socialos.conf" /opt/socialos/systemd/containerd.service.d/
cd /opt/socialos && docker compose config -q && echo compose-ok
docker compose config --format json | jq -c '[.services[] | [.cgroup_parent, .memswap_limit == .mem_limit, .pids_limit]] | unique'
```

Verify: `compose-ok`, then `[["socialos.slice",true,128]]`. `docker compose ps` still shows the old containers, healthy.
Irbisa check. Rollback: restore the files from the step-0 archive, `systemctl start socialos-autoupdate.timer`.

## Step 3: move the SocialOS containers into the slice (risk: low; SocialOS restarts, about 1 minute)

Recreates every SocialOS container with `cgroup_parent: socialos.slice`, no swap and a process cap. Irbisa is not touched;
the restart burst itself already runs inside the slice caps. Pick a quiet moment.

```bash
cd /opt/socialos
./deploy.sh "$(cat .deploy/current_tag)" --no-migrate     # same tag: up -d, waits for /ready, checks the public URLs
for c in $(docker compose ps -q); do docker inspect -f '{{.Name}} {{.HostConfig.CgroupParent}} {{.HostConfig.Memory}} {{.HostConfig.MemorySwap}} {{.HostConfig.PidsLimit}}' "$c"; done
cat /sys/fs/cgroup/socialos.slice/{memory.max,memory.high,memory.swap.max,pids.max,cpu.max,io.max}
systemctl status socialos.slice --no-pager | head -8
```

Verify: every container shows `socialos.slice`, Memory equal to MemorySwap, PidsLimit 128; the slice files show
`696254464`, `645922816`, `0`, `512`, `100000 100000` and `8:0 rbps=62914560 wbps=max riops=max wiops=max`; `deploy.sh` ended with
`DEPLOYED`; `systemctl show socialos.slice -p MemoryCurrent` is about 300-400 MB; `ls /sys/fs/cgroup/system.slice | grep
docker-` prints nothing; `docker compose exec backend env | grep GOMEMLIMIT` says `100MiB`. Irbisa check. Watch for
10 minutes: `journalctl -k --since -10min | grep -i oom` empty.

Then the load check (needs the owner's OK for the test account and the test files; run it from outside the server, at a
quiet time): 20 parallel logins and 3 parallel 100 MB media uploads against `https://app.<domain>`. During and after it:

```bash
docker stats --no-stream --format '{{.Name}} {{.MemUsage}} {{.MemPerc}}'      # peak of each container <= 70% of its limit
for c in $(docker compose ps -aq); do docker inspect -f '{{.Name}} OOMKilled={{.State.OOMKilled}}' "$c"; done   # all false
systemctl show socialos.slice docker containerd -p MemoryPeak
irbisa_check
```

`docker stats` shows the current value only: sample it every few seconds during the test
(`while sleep 2; do docker stats --no-stream ...; done`) or read each container's `memory.peak` in
`/sys/fs/cgroup/socialos.slice/docker-<id>.scope/`. A container above 70% of its limit, an `OOMKilled=true` or a slice
`MemoryPeak` near 664M means a cap must be raised (and the trade-off in README section 16 revisited) before going on.

Rollback (back to the old placement): `echo 'SOCIALOS_CGROUP_PARENT=' >>.env && docker compose up -d --wait`, or restore
the step-0 archive and `docker compose up -d --wait`.

## Step 4: cap dockerd and containerd (risk: low; no restart)

Resource settings only. The drop-ins deliberately add **no** `After=`/`Before=` against Caddy or Irbisa: if one of those
were ever ordered after Docker, an ordering there would form a boot cycle and systemd could drop the other site's start job.

```bash
install -d -m 755 /etc/systemd/system/docker.service.d /etc/systemd/system/containerd.service.d
install -m 644 /opt/socialos/systemd/docker.service.d/socialos.conf /etc/systemd/system/docker.service.d/socialos.conf
install -m 644 /opt/socialos/systemd/containerd.service.d/socialos.conf /etc/systemd/system/containerd.service.d/socialos.conf
systemctl show irbisa caddy -p NeedDaemonReload --value   # still "no" twice
systemd-analyze verify docker.service containerd.service 2>&1 | grep -i cycle   # must print nothing
systemctl daemon-reload
cat /sys/fs/cgroup/system.slice/docker.service/{cpu.max,memory.high} /sys/fs/cgroup/system.slice/containerd.service/{cpu.max,memory.high}
systemctl show docker containerd -p MemoryCurrent,MemoryPeak
```

Verify: `25000 100000`, `201326592`, `25000 100000`, `134217728`. If a cgroup file still shows `max`, apply the same
values to the running units without a restart:
`systemctl set-property --runtime docker.service CPUQuota=25% CPUWeight=50 MemoryHigh=192M` and
`systemctl set-property --runtime containerd.service CPUQuota=25% CPUWeight=50 MemoryHigh=128M`.
`docker ps` and `docker compose ps` answer as before. Irbisa check. After the next deploy, `systemctl show docker
containerd -p MemoryPeak` again: a peak far above `MemoryHigh` means the daemon was throttled (and may have swapped); raise
the value in a drop-in and report it. After the next reboot: `journalctl -b | grep -i 'ordering cycle'` prints nothing.
Rollback: `rm /etc/systemd/system/{docker,containerd}.service.d/socialos.conf && systemctl daemon-reload`; if the cgroup
files keep the values, `systemctl set-property --runtime docker.service CPUQuota= MemoryHigh=infinity` (same for containerd).

## Step 5: Caddy start pre-check (risk: low; Caddy is not restarted or reloaded)

```bash
install -m 644 /opt/socialos/systemd/socialos-caddy-precheck.service /etc/systemd/system/
systemctl daemon-reload
systemctl enable socialos-caddy-precheck.service      # adds caddy.service.wants/ symlink: a weak Wants, ordering Before=caddy
systemctl start socialos-caddy-precheck.service       # one run now, while Caddy keeps running
journalctl -u socialos-caddy-precheck -n 5 --no-pager
ls /opt/socialos/caddy/                                # socialos.caddy still there, no quarantine/
```

Verify: the journal says "is valid"; `systemctl show caddy -p Wants` lists `socialos-caddy-precheck.service`; `systemctl
is-active caddy` is still `active` with the same `ActiveEnterTimestamp` as before. Irbisa check.
Rollback: `systemctl disable socialos-caddy-precheck.service && rm /etc/systemd/system/socialos-caddy-precheck.service && systemctl daemon-reload`.

## Step 6: the guard (risk: low; it can only stop and restart SocialOS containers, and only under pressure that SocialOS adds to)

```bash
cd /opt/socialos
printf '\nGUARD_HEALTH_URLS="http://127.0.0.1:3001/ https://irbisa.com/"\n' >>.env
./host-proxy/socialos-guard.sh --dry-run               # expect: "level 0 (... contributor: false); dry run, nothing changed"
install -m 644 systemd/socialos-guard.service systemd/socialos-guard.timer /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now socialos-guard.timer
systemctl start socialos-guard.service && journalctl -u socialos-guard -n 5 --no-pager
./host-proxy/socialos-guard.sh --status
systemctl start socialos-autoupdate.timer              # stopped in step 2
```

Verify: the journal line says `level 0`; `--status` shows no shed level and no alerts (an alert `slice` means step 3 did not
take); `systemctl list-timers socialos-guard.timer socialos-autoupdate.timer` shows both. `docker compose ps`: all running.
Irbisa check.
Rollback: `systemctl disable --now socialos-guard.timer && ./host-proxy/socialos-guard.sh --resume` (starts what it
stopped), then remove the two unit files and `daemon-reload`.

## Step 7 (optional, needs the owner's OK): Irbisa in the uptime checks

`UPTIME_URLS` replaces the default targets of `.github/workflows/uptime.yml`, so list ours too:

```bash
gh variable set UPTIME_URLS --body "https://api.<domain>/ready https://app.<domain>/login https://mcp.<domain>/health https://irbisa.com/"
gh workflow run uptime.yml
```

Alert only: an outage opens an `incident` issue in this repository. Rollback: `gh variable delete UPTIME_URLS`.

## Afterwards

- One day later: `systemctl show socialos.slice -p MemoryCurrent,MemoryPeak`, `journalctl -k | grep -i oom`,
  `./host-proxy/socialos-guard.sh --status`, `journalctl -u socialos-guard -p warning --since -1d`.
- Not covered here (owner decisions): `caddy.service` has `Restart=no` (a Caddy crash takes Irbisa down until someone
  starts it); journald may grow to 4 GB (no `SystemMaxUse`); SocialOS backups are not scheduled yet (`backup.sh` exists).

## Risk table (audit 2026-10-09)

| Risk | Likelihood | Impact on Irbisa | Mitigation after these steps | Gap |
|---|---|---|---|---|
| SocialOS memory leak / runaway | medium | swap thrash, OOM of Irbisa | per-container caps, no swap, slice 664M hard, oom_score_adj 500, guard (only if SocialOS contributes) | caps overcommit the slice (784m vs 664M): a SocialOS OOM, never Irbisa's |
| CPU spin in a container | medium | slower responses | slice quota 1 CPU, weight 50; guard on Irbisa CPU stall | - |
| Fork bomb | low | no new threads host-wide (threads-max 15098, was 2264 per container) | pids_limit 128, slice TasksMax 512 | - |
| Log flood | low | dockerd CPU and disk IO | json-file 5x10 MB per container, dockerd CPUQuota 25% | - |
| Image pull / deploy burst | high (each release) | CPU, IO, page cache | containerd/dockerd caps, autoupdate skipped while shed | IO caps on the daemons not set |
| Disk IO saturation | low | slow SQLite | slice read cap 60 MB/s, guard on Irbisa IO stall and SocialOS IO rate | no write cap until the disk is measured (journal-commit stalls) |
| DB bloat / MinIO fills the disk | low (now 0.1 GB) | full disk = Irbisa write errors | alerts at 15 GB data and 80% disk | no hard filesystem quota |
| Host OOM | low | Irbisa killed | SocialOS capped at about 50% and picked first | - |
| Broken SocialOS Caddy snippet at Caddy start | low | Caddy does not start: Irbisa down | precheck quarantines it before start | reload path already safe |
| `/opt/socialos/caddy` deleted | low | none (empty glob is allowed) | - | - |
| Let's Encrypt or sslip.io outage | medium | none (own hostnames, per-hostname limits) | - | - |
| Port conflict | low | Irbisa cannot bind :3001 | SocialOS on 127.0.0.1:13000/13333/18080/19000 | - |
| Docker daemon crash or upgrade | low | none | live-restore; limits are kernel cgroups, independent of dockerd | docker/caddy are not auto-upgraded (third-party repos) |
| Host reboot | certain, eventually | slow boot | slice applies at container start; no ordering against Irbisa (a cycle could drop its start job) | - |
| Failed deploy | medium | none | deploy.sh rolls back, inside the slice | - |
| Pressure caused by Irbisa, apt or journald | medium | none from SocialOS | the guard alerts but stops nothing unless SocialOS contributes | - |
| Caddy crash | low | Irbisa down | - | `caddy.service` has `Restart=no` (owner) |
