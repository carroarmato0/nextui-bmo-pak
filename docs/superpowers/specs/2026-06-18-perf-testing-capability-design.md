# Perf-Testing Capability — bmo-pak Design

**Date:** 2026-06-18
**Status:** Approved (design)

## Problem

BMO's recurring failure mode on-device is **OOM** (idle full-set OOM, animation
budget tuning, multi-buffer present). We currently have no way to observe CPU or
memory usage on the device over a session. We need a toggleable perf-testing
capability and a repeatable workflow to collect data and identify the highest-
impact fixes.

The critical constraint: **Go's `pprof` heap profile only sees Go-managed
allocations.** It does not see SDL textures/surfaces or the CGO heap, which is
likely the dominant share of BMO's resident memory. Any memory story for BMO
must therefore include an OS-level (RSS) view, not just `pprof`.

## Approach

Mirror the proven Itch-io shape — a `.profile-flags` file on the device that
`launch.sh` injects ahead of `"$@"`, toggled by a `scripts/debug.sh` helper —
and **extend** it with the BMO-specific pieces that matter for OOM:

1. Go `pprof` (CPU + heap, file-dump and live HTTP) for Go-side attribution.
2. An **in-process RSS/CPU sampler** that captures the whole-process footprint
   over time and survives the OOM kill.

**Rejected alternative:** baking a `BMO_PROFILE` env var into `launch.sh`. The
file approach wins because `debug.sh` writes/removes it without editing the
script, it survives NextUI relaunches, and `profile-restore` is a clean
one-liner.

## Components

### 1. `internal/perf` package (pure-Go, testable under `CGO_ENABLED=0`)

Self-contained, no SDL/CGO dependency, so it builds and tests without CGO.

**pprof glue** (`pprof.go`):
- `StartCPUProfile(path) (stop func(), err error)` — opens the file, starts the
  CPU profile, returns a stop func that stops the profile and closes the file.
- `WriteHeapProfile(path) error` — `runtime.GC()` then `pprof.WriteHeapProfile`.
- `StartLiveServer(addr, logger)` — runs `net/http` with the `net/http/pprof`
  handlers in a goroutine; errors logged, never fatal.

**Sampler** (`sampler.go`):
- `Sampler` struct created with an output path, interval, a `StateFunc`
  (returns the current assistant-state string), and a logger.
- `Start(ctx)` launches a goroutine that, every `interval`:
  - reads `/proc/self/status` → `VmRSS`
  - reads `/proc/self/stat` → `utime`+`stime`, diffs against the previous
    sample and the wall-clock delta to compute CPU %
  - calls `runtime.ReadMemStats` for Go `HeapAlloc`, `HeapSys`, `NumGC`,
    and `runtime.NumGoroutine()`
  - appends one CSV row and **flushes** it (so the file is complete up to the
    last sample even if the process is OOM-killed)
- `Stop()` writes a final row and closes the file.
- Parsing is factored into pure functions that take an `io.Reader`
  (`parseVmRSSKB`, `parseProcStatCPU`) for table-driven tests.

**CSV schema** (`perf-sample.csv`):

```
uptime_s,state,vmrss_kb,go_heapalloc_kb,go_heapsys_kb,go_numgc,cpu_pct,goroutines
```

- `state` is the current assistant state (idle / listening / thinking /
  speaking / idle-anim), supplied by `StateFunc`. This makes the CSV
  self-segmenting: an RSS spike can be attributed to a specific state.
- **`vmrss_kb` minus `go_heapsys_kb` ≈ the CGO/SDL/texture footprint** that
  `pprof` cannot show — the key OOM signal.

### 2. `cmd/bmo-pak/main.go` flag wiring

- Add `flag` parsing at the top of `run()`:
  - `-cpuprofile <file>` — write CPU profile to file
  - `-memprofile <file>` — write heap profile on exit
  - `-pprof <addr>` — serve live pprof (e.g. `:6060`)
  - `-perfsample <file>` — write the RSS/CPU CSV
  - `-perfinterval <dur>` — sampler interval (default `2s`)
- All flags empty ⇒ no goroutines started, **zero overhead**, normal launch.
- CPU profile + sampler start early in `run()`. The sampler's `StateFunc` reads
  an `atomic.Value`/`atomic.Int32` that the face loop updates as the assistant
  state changes (one cheap store per transition).
- Heap-profile write and final sampler flush happen in the cleanup **after** the
  face loop exits — the same graceful path that already presents black 3× on
  SIGTERM. (Only the graceful SIGTERM path runs; `kill -9` loses the final
  flush, which is why each sampler row is flushed as it is written.)

### 3. `launch.sh`

Insert, before the final `exec`:

```sh
PROFILE_FLAGS=""
if [ -f "$PAK_DIR/.profile-flags" ]; then
    PROFILE_FLAGS="$(cat "$PAK_DIR/.profile-flags")"
fi
# shellcheck disable=SC2086
exec "$PAK_DIR/bin/$PLATFORM/bmo-pak" $PROFILE_FLAGS "$@"
```

Only the repo `launch.sh` is edited; `scripts/release.sh` copies it into the
dist pak directories, so the stale `dist/` copies are regenerated, not edited.

### 4. `scripts/debug.sh` (new)

Subcommands mirroring Itch-io, with BMO device paths and the reliable kill
recipe from CLAUDE.md. Platform defaults to `tg5040`, overridable.

- `profile` — write `.profile-flags` enabling cpu+mem+sample
- `profile-cpu` — cpu only
- `profile-mem` — heap only
- `profile-sample` — RSS/CPU CSV only
- `profile-live` — `-pprof :6060` + `adb forward tcp:6060 tcp:6060`
- `profile-restore` — remove `.profile-flags` (and any port forward)
- `pull-profile` — pull `*.prof` + `perf-sample.csv` to `./debug-profiles/`

Device paths: pak at `/mnt/SDCARD/Tools/<platform>/BMO.pak`, profiles written to
`/tmp/` on device, logs at `/mnt/SDCARD/.userdata/<platform>/logs/`.

### 5. Skill `.claude/skills/bmo-pak-profiling/SKILL.md`

Drives the end-to-end loop:

1. Enable flags via `debug.sh profile`.
2. Launch via NextUI (perf flags only take effect on a NextUI launch, not a
   manual ADB launch — manual launch masks the real config path).
3. Exercise BMO's workloads in sequence, noting timestamps: idle, idle
   face-cycling, listening/thinking, speaking, whistle/look_around idle anims.
4. Stop (exit BMO gracefully so the heap profile + final CSV row flush).
5. `debug.sh pull-profile`.
6. Analyze: `go tool pprof bin/<platform>/bmo-pak debug-profiles/bmo-cpu.prof`
   for CPU; heap profile for Go allocs; **parse the CSV** for RSS-over-time and
   the VmRSS−HeapSys gap segmented by state.
7. Write `docs/profiling-findings-<date>.md` with **Action Items ranked by
   impact**.

The skill **stops at the findings doc** (ranked action items) and explicitly
offers to implement the top fixes — it does not implement them automatically.
Each fix is a separate, user-approved follow-up.

### 6. `docs/profiling-findings-<date>.md` template

Matches the Itch-io format: environment, workloads exercised, CPU findings, heap
findings, RSS-over-time findings (with the CGO/SDL gap), and a ranked **Action
Items** section.

## Data Flow

```
debug.sh writes .profile-flags  ──►  launch.sh reads it  ──►  bmo-pak flags
        │                                                          │
        │                                            CPU/heap .prof + perf-sample.csv
        │                                            (device /tmp, flushed per row)
        ▼                                                          ▼
debug.sh pull-profile  ◄──────────────────────────────────  pulled to host
        │
        ▼
go tool pprof + CSV analysis  ──►  docs/profiling-findings-<date>.md (ranked fixes)
```

## Error Handling

- `/proc` read/parse failure: log once, sampler continues (don't spam the log).
- pprof file open / write error: logged, non-fatal — profiling must never crash
  a normal session.
- Live pprof server error: logged in its goroutine, non-fatal.
- All perf machinery is inert when its flag is empty.

## Testing

- Table-driven unit tests for `parseVmRSSKB` and `parseProcStatCPU` (fixtures of
  real `/proc/self/status` and `/proc/self/stat` text).
- CPU%-delta computation across two samples.
- CSV row formatting (schema/order/units stable).
- Smoke test: `StartCPUProfile` then stop writes a non-empty, parseable file.
- `golangci-lint run ./...` clean; perf package builds under `CGO_ENABLED=0`.

## Out of Scope

- Implementing the fixes the first profiling run surfaces (separate follow-ups).
- Non-Linux sampling (dev and device are both Linux; `/proc` is assumed).
- Continuous/always-on profiling — perf is opt-in via flags only.
