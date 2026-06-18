# Profiling Findings — bmo-pak (2026-06-18)

## Environment

- Device / platform: **TrimUI Brick** (`4c000c820706472229d`), platform tg5040, fbdev/mali, `displayMode=1024x768`. (The Smart Pro `1c00…81e5d` disconnected before pull; not profiled this round.)
- Framebuffer resolution: 1024×768 @ 32bpp
- Build / commit: `feat/perf-testing` merged to main (`25bedd6`), binary Build ID `ccc6c219…`
- Flags used: `-cpuprofile -memprofile -perfsample` (2s interval)
- Session: 304 s; mode=ai, trigger=ptt. Two PTT exchanges (STT→Chat→TTS both succeeded), one proactive-remark failure (no network → 5 s auto-recover).

## Workloads exercised

| Workload             | uptime_s range | Notes |
|----------------------|----------------|-------|
| idle                 | 0–304 (dominant)| 126 of 154 samples; includes warmup |
| idle animations      | interleaved     | whistle / look_around / neutral time-driven |
| listening + thinking | ~50, ~254       | PTT press → STT |
| speaking             | ~43, ~265       | TTS playback (428 KB, 560 KB) |
| error (environmental)| ~204            | proactive remark failed, OpenAI unreachable; auto-recovered |

## CPU findings

`go tool pprof -cum` over 304 s; total samples 172.7 s = **~57 % of one core, sustained** (steady idle ≈ 46–48 %, warmup spikes 350–380 % across cores in the first ~8 s).

| Path | cum | % |
|---|---|---|
| `renderer.Draw` | 126.4 s | 73.7 % |
| └ `renderer.present` → `sdl.Texture.Update` (`SDL_UpdateTexture`, cgo) | 55.1 s | 31.9 % |
| └ `runtime.memmove` (pixel copy into texture) | 39.2 s | 22.7 % |
| └ `face.Rasterize` → `oksvg.SvgIcon.Draw` | 36.6 s | 21.2 % |

**The render path is ~74 % of all CPU, and ~49 % of total CPU is just uploading + presenting the framebuffer texture every frame** (`SDL_UpdateTexture` + `memmove`). `present()` (`internal/renderer/bmo.go:348`) calls `tex.Update(...)` + `Clear` + `Copy` + `Present` **unconditionally on every `Draw`, with no dirty/changed check** (`bmo.go:314-346`). At idle the face pixels are static (only the corner clock changes, once/minute, and time-driven idle anims occasionally), so the vast majority of these full-frame ~3 MB uploads are redundant. This is the sustained ~46 % idle CPU — pure battery/heat cost.

## Heap findings (Go allocations)

`inuse_space` at exit is only 3.2 MB (the heap profile is written after `runtime.GC()` on graceful shutdown, so it does not reflect steady state). The signal is in **`alloc_space` (cumulative)**:

| Allocator | cum | % |
|---|---|---|
| `face.Rasterize` (total) | 806 MB | 93.6 % |
| └ `image.NewRGBA` (destination frame) | 201.5 MB | 23.4 % |
| └ `vector.Rasterizer.accumulateMask` (scratch) | 201.5 MB | 23.4 % |
| └ `vector.Rasterizer.setUseFloatingPointMath` (scratch) | 201.5 MB | 23.4 % |

Only **72 rasterizations happened this session** (from the log), so each `face.Rasterize` allocates **~11 MB** of transient memory: a fresh full-res `image.NewRGBA` plus two `vector.Rasterizer` scratch masks, none reused across calls. This churn — not a leak — is what drives GC and pushes `HeapSys` to its high-water mark.

## RSS-over-time findings (whole process)

From `bmo-perf-sample.csv` (155 rows, 2 s interval):

- **Peak VmRSS: 323,728 kB (~316 MB)** at t≈138 s (idle). Start: 12 MB. **+304 MB growth, almost all within the first ~28 s, at idle.**
- **`HeapSys` climbs to 298,368 kB (~291 MB) and never returns to the OS** — Go's runtime keeps the high-water mark. So Go heap, not textures, dominates RSS.
- **Non-Go footprint (VmRSS − HeapSys): peak ~25 MB**, avg ~14 MB. SDL textures / CGO are a *small* slice — the original "it's probably SDL textures" hypothesis is **disproved** for this workload.
- Steady-state **live** heap (`go_heapalloc` before the shutdown GC): ~265 MB — i.e. ~88 full-res 1024×768 ARGB frames retained.
- Leak check: RSS is stable across repeated idle/speak cycles (no unbounded growth) — but the **retained cache is the OOM driver**: `face.Cache.frames` (`internal/face/cache.go:12`) is an **unbounded `map[string][]uint32` of full-res ARGB frames, never evicted, no cap**. Every distinct expression (and every animation frame of it) ever shown stays resident. On a fuller face set / more constrained unit this is the documented "Idle full-set OOM".

## Action Items (ranked by impact)

> **✅ DONE & verified on-device (2026-06-18, Brick).** Implemented as two commits
> on `fix/renderer-dirty-present`: (1) skip `present()` when the framebuffer is
> byte-identical to the last presented frame (presenting `swapChainDepth`× after
> a change to keep multi-buffered backends consistent); (2) skip the entire
> `Draw` rebuild for static frames (output determined by expression+size).
> **Measured idle CPU on a static face: ~46% → ~1.5–2% (~25×).** listening
> 38%→10%, thinking 36%→4.5%. Animated faces (whistle/look_around) and speaking
> lip-sync correctly still rebuild+present every frame. Item #2 (cache cap) and
> #3 (rasterizer pooling) remain open — RSS is unchanged (~300 MB peak).

1. **[highest impact — CPU/battery/heat] Skip the texture upload + present when the frame is unchanged.**
   Evidence: `SDL_UpdateTexture` (32 %) + `memmove` (23 %) = ~49 % of CPU; `present()` at `internal/renderer/bmo.go:348` uploads and presents unconditionally every `Draw`. Fix: dirty-track `r.pixels` (cheap rolling hash, or a `dirty` flag set by the draw primitives) and skip `tex.Update`+`Clear`+`Copy`+`Present` when identical to the last presented frame. The corner clock (once/min) and time-driven idle anims naturally mark themselves dirty, so animation is unaffected. Expected win: idle CPU from ~46 % toward single digits; large battery/thermal improvement. Mind the multi-buffer-on-exit rule ([[reference_blank_on_exit_multibuffer]]) — keep the 3× black-present on shutdown.

> **✅ DONE (2026-06-18) on `fix/bound-face-cache`.** The static `Cache` now pins
> the canonical set (fixed built-in list, always hot via the idle rotation —
> never evicted, so the default config is unchanged) and LRU-evicts only
> non-canonical mod faces past `staticCacheModBudget` (12 frames ≈ 36 MiB).
> A mod with arbitrarily many custom `.svg` faces can no longer grow static
> residency without bound; cold custom faces re-rasterize on demand. (The
> animation engine was already bounded by `animMemoryBudget` = 128 MiB.)

2. **[OOM driver — memory] Bound the face frame cache.**
   Evidence: `face.Cache.frames` (`internal/face/cache.go:12`) is unbounded and never evicted; ~265 MB live ≈ 88 retained full-res frames; `HeapSys` plateau at 291 MB. Fix: cap the cache (LRU by recency) and/or retain only the active expression's frames plus a small idle set, evicting inactive animated-face frame-sets. Aligns with [[reference_idle_full_set_oom]] ("serve amplitude faces from the static cache; only animate when speaking or time-driven"). Expected win: peak RSS down by 150–250 MB, removing the OOM headroom problem.

3. **[GC pressure — memory churn, some CPU] Pool the rasterizer and destination buffer.**
   Evidence: 806 MB `alloc_space` over only 72 rasterizations = ~11 MB/call; `image.NewRGBA` + two `vector.Rasterizer` scratch masks allocated fresh each time (`internal/face/Rasterize`). Fix: reuse a per-size scratch `*image.RGBA` and a pooled `vector.Rasterizer` (`sync.Pool`) across rasterizations. Expected win: ~90 % less rasterization allocation, lower `HeapSys` high-water mark, smoother GC. Smaller CPU win than #1 but compounds with #2.

### Notes / non-issues
- The "error" state at t≈204 s and the 5 voice-pipeline warnings are **environmental** (no/blocked network to `api.openai.com`: TLS cert, DNS, 429 quota). Auto-recovery (5 s) worked correctly. Not a perf defect.
- Profiling overhead is negligible (CPU profiler buffer ~1.2 MB; sampler is a 2 s `/proc` read).
- Only the Brick was captured; re-run on the Smart Pro (multi-buffered EGL) to confirm #1's win there, where redundant presents are even costlier.
