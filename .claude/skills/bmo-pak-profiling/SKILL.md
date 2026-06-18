---
name: bmo-pak-profiling
description: Use when profiling bmo-pak CPU/memory on a live TrimUI device — enabling on-device profiling flags, exercising BMO's workloads, pulling pprof + RSS/CPU samples, and writing a ranked findings doc. BMO's recurring failure is OOM, and the RSS sample exposes the SDL/CGO memory that Go's pprof cannot see.
---

# BMO Pak — On-Device Profiling

Toggleable profiling lives behind a `.profile-flags` file that `launch.sh`
injects. `scripts/debug.sh` toggles it over ADB. The binary supports:
`-cpuprofile`, `-memprofile`, `-pprof :addr`, `-perfsample <csv>`,
`-perfinterval <dur>`.

**Key insight:** Go's heap profile only sees Go allocations. The RSS sampler's
`vmrss_kb` minus `go_heapsys_kb` is the SDL-texture/CGO footprint — usually the
real driver of BMO's OOM. Always read the CSV, not just the heap profile.

## Prerequisites

- Device connected via ADB (`adb devices` lists it). ADB enabled in NextUI
  Settings → Developer.
- A current build deployed: `./scripts/deploy.sh`.

## Collection workflow

1. Enable profiling (pick one):
   - `./scripts/debug.sh profile`        — CPU + heap + RSS sample (default)
   - `./scripts/debug.sh profile-sample` — RSS/CPU CSV only (lowest overhead)
   - `./scripts/debug.sh profile-live`   — live pprof on :6060
2. **Launch BMO via NextUI** (not a manual ADB launch — a manual launch reads a
   phantom config path and won't reflect real behaviour).
3. Exercise workloads in sequence, noting wall-clock times so CSV rows can be
   segmented by state:
   - idle (default face), ~30s
   - idle long enough to trigger idle animations (whistle / look_around)
   - listening + thinking (push-to-talk a query)
   - speaking (let a TTS reply play fully)
4. **Exit BMO gracefully** (B/BTN_SOUTH to exit) so the heap profile and final
   CSV row flush. A `kill -9` loses the heap profile and the last sample.
5. `./scripts/debug.sh pull-profile`  — pulls to `./debug-profiles/`.
6. `./scripts/debug.sh profile-restore`.

## Analysis

- CPU: `go tool pprof -top -nodecount=20 bin/tg5040/bmo-pak debug-profiles/bmo-cpu.prof`
  (use `-cum` for cumulative). `-list <func>` to see hot lines.
- Heap: `go tool pprof -top -sample_index=inuse_space bin/tg5040/bmo-pak debug-profiles/bmo-mem.prof`.
- RSS over time: read `debug-profiles/bmo-perf-sample.csv`. Columns:
  `uptime_s,state,vmrss_kb,go_heapalloc_kb,go_heapsys_kb,go_numgc,cpu_pct,goroutines`.
  - Plot/scan `vmrss_kb` per `state`: which state drives peak RSS?
  - Compute `vmrss_kb - go_heapsys_kb` = non-Go (SDL/CGO) memory. A large or
    growing gap points at textures/surfaces, not Go allocations.
  - Rising `vmrss_kb` across repeated cycles of the same state = a leak.

## Output

Write `docs/profiling-findings-<YYYY-MM-DD>.md` from
`docs/profiling-findings-TEMPLATE.md`. Fill every section. End with an
**Action Items** list ranked by impact (highest-RSS/CPU win first), each with
the evidence (function, line, or CSV observation) that justifies it.

**Stop after the findings doc.** Do not implement fixes automatically — present
the ranked action items and offer to implement the top ones as separate,
user-approved changes.
