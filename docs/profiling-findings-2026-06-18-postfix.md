# Profiling Findings — bmo-pak, post-fix verification (2026-06-18)

Re-run after landing all three baseline action items (dirty-present, bounded
face cache, pooled rasterizer). Compared against the original
[2026-06-18 baseline](profiling-findings-2026-06-18.md). Raw artifacts:
`debug-profiles/brick-postfix-2026-06-18/`.

## Environment

- Device / platform: **TrimUI Brick** (`4c000c820706472229d`), platform tg5040, fbdev/mali, 1024×768 @ 32bpp.
- Build: current `main` `8e73c73` (fixes #1 present-skip, #2 bounded cache, #3 pooled rasterizer), Build ID `e1eb7275…`.
- Flags: `-cpuprofile -memprofile -perfsample` (2 s interval). Session 304 s, default config (no mod faces).
- Workloads: idle (dominant), idle animations (whistle / look_around), one PTT exchange (listening→thinking→speaking, TTS played fully), graceful exit.

## Headline comparison

| Metric | Baseline | Post-fix | Change |
|---|---|---|---|
| Total CPU (% of one core over 304 s) | 56.8 % (172.7 s) | **23.3 %** (70.8 s) | **−59 %** |
| Static idle CPU | ~46 % | **~2 %** (idle median 4 %, p25 2 %, p10 1.5 %; 87/136 idle samples <5 %) | **~20–25×** |
| Peak VmRSS | 323,728 kB (~316 MB) | **285,816 kB (~279 MB)** | **−37 MB** |
| HeapSys plateau | ~291 MB | **~255 MB** (261,536 kB) | **−36 MB** |
| `Rasterize` scratch alloc — `accumulateMask` | 201.5 MB | **21.0 MB** | **−90 %** |
| `Rasterize` scratch alloc — `setUseFloatingPointMath` | 201.5 MB | **21.0 MB** | **−90 %** |
| `Rasterize` dest alloc — `image.NewRGBA` | 201.5 MB | **21.0 MB** | **−90 %** |
| Render present (`SDL_UpdateTexture` + `memmove`) | ~49 % of CPU | dropped out of top set | eliminated at idle |

## What each fix did, confirmed

**#1 dirty-present + rebuild-skip — the big CPU/battery win.** Static idle
collapsed to ~2 % (the `cpu_pct` floor across all quiet idle windows, e.g.
t=96–114 s sat at 2–4 %). The unconditional `SDL_UpdateTexture` + `memmove`
that was ~49 % of baseline CPU no longer appears in the hot set: at a static
frame `Draw` early-returns before both the rebuild and the present.

**#3 pooled rasterizer — the allocation/GC win.** The three per-call scratch
allocations each fell from ~201 MB cumulative to ~21 MB (−90 %), exactly as
designed — the residual is the rebuild path (size change / pool miss / GC
dropping a pooled ctx). This pulled the `HeapSys` high-water mark down ~36 MB,
which is most of the −37 MB peak-RSS improvement.

**#2 bounded cache — not exercised this run.** Default config uses only the
canonical face set, which the cache *pins*, so there were no mod faces to evict.
Its protective effect (capping an unbounded mod-face axis) needs a run with a
mod loaded to show up in numbers — worth repeating once the new mod exists.

## Remaining costs (all expected / by design)

- **Idle animations (whistle / look_around): ~43 % CPU bursts** (e.g. t=90–94 s
  held 43.5 %, then dropped to 2 % when the face returned to static neutral).
  These rebuild + present every frame by design; the present-skip correctly does
  *not* apply while pixels are changing.
- **Speaking: ~44–49 % CPU** (lip-sync rebuilds + presents every frame). Same as
  baseline by design.
- **Warmup: 340–360 % across cores for ~8 s** while `Cache.Warm` rasterizes the
  canonical set (CPU profile: `Cache.Warm`→`warmFrame` 25 %, animation
  `buildFrames` 21 %). One-time startup cost, unchanged.

## New observation — next candidate (low priority)

The content-based dirty check introduced by #1 is now itself visible:
`shouldPresent` → `bytes.Equal` → `runtime.memequal` is **15.65 % of total
CPU**. It is only paid on *changing* frames (animations, speaking, the
once-a-minute clock) — static idle skips it via the rebuild-skip, which is why
idle is ~2 %. If this becomes a target, replacing the full-frame compare with a
dirty flag set by the draw primitives would remove it, but at ~16 % of a
23 %-of-one-core total it is far below the original present cost and not urgent.

## Action items

No new fixes recommended. All three baseline items are verified effective on
hardware. Optional future work, in priority order:

1. Re-profile **with a mod loaded** to quantify #2 (bounded cache) and confirm
   custom faces don't reintroduce the OOM axis.
2. Re-profile on the **Smart Pro** (multi-buffered EGL) — not captured this
   round; its present-skip win should be even larger (3 presents/change vs 1).
3. If idle-animation / speaking CPU ever matters: replace the full-frame
   `bytes.Equal` dirty check with a primitive-level dirty flag.
