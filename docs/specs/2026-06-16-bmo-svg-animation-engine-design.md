# BMO Declarative SVG Animation Engine — Design Spec

**Date:** 2026-06-16
**Status:** Approved (design)
**Builds on:** `2026-06-15-bmo-mods-foundation-design.md` (the `mods/` foundation,
`mod.json`, and the `SelfContained()` rule) and the existing `internal/face`
render/cache pipeline.

## Motivation

BMO has exactly one animated face — `speaking` — and it is entirely hardcoded in
Go. `internal/face/speak.go` procedurally computes mouth geometry from an openness
value `t ∈ [0,1]`, renders the `speaking.svg` Go-template at 6 discrete levels, and
the renderer special-cases `if Canonical(expr) == ExprSpeaking` to pick a level
from `SpeakAmplitude` (with a sine fallback) and blit a base frame plus a mouth
"strip". Nothing else can animate, and a mod author cannot describe motion at all.

This spec introduces a **declarative SVG animation engine**: animations are
described as data (in `mod.json` and an embedded default table), keyed by
expression name, and rendered by a general engine. It must be easy for both humans
and agents to author, flexible enough for tiny looping animations, and cleanly
hookable to lip-sync. As proof, the built-in `speaking` animation is **re-expressed
through the new engine** and the bespoke `speak.go` machinery is deleted.

## Goals & Non-Goals

**Goals**
- Declarative animation authoring (no code) for mods, keyed by expression name.
- Two authoring styles behind one interface: an explicit **frame list** of SVG
  stills, or a **parametric SVG template** sampled at N steps.
- Two drivers: **time** (clock-driven) and **amplitude** (lip-sync).
- A **swappable signal interface** so future audio-derived inputs (visemes, pitch)
  replace amplitude without engine or schema changes.
- **Dogfood:** reproduce the current `speaking` animation through the engine and
  remove the hardcoded path.
- Static single-SVG faces keep working unchanged.

**Non-Goals (deferred)**
- Richer audio signals (visemes/pitch) — the interface is ready; no impl ships.
- Inter-frame interpolation/easing — the engine snaps to discrete steps.
- Cross-expression transitions/blending.
- Native SMIL/`<animate>` SVG animation — the device rasterizer (oksvg) cannot
  render it.

## Architecture

A new animation concern lives in `internal/face`, alongside the existing `Cache`.
An *animation* is a named, declarative unit keyed by an **expression name**
(`speaking`, `sparkle`, a mod's `grumpy_talk`…). Each tick it produces a **frame
index** from a **driver**, and a **frame provider** turns that index into a full
ARGB buffer.

Three small, independently testable units:

1. **Frame provider** — given a step `i ∈ [0, steps)`, returns the SVG bytes for
   that step. Two implementations:
   - *frame-list*: loads `name_i.svg` from the library (override-then-embedded,
     reusing `Library.Bytes`).
   - *parametric template*: renders a Go `text/template` SVG with the named param
     set to a value sampled linearly across `[from, to]` over `steps`.
2. **Driver** — given `(clock, epoch, signal)`, returns the current step index.
   Two implementations: *time* and *amplitude* (below).
3. **Engine** — owns rasterization plus a **scoped** frame cache (only the active
   expression's frames are resident) and the per-tick lookup. Falls back to the
   existing static `Cache.Frame` when an expression has no animation.

### Rendering strategy: full frames + scoped warming

Each animation step is rasterized to a **full ARGB buffer**, blitted exactly like
every static face already is (the render loop already `copy`s a full buffer per
tick). This lets us delete the bespoke strip/band/template-sampling code. Memory is
bounded not by per-pixel diffing but by **scope**: only the *active* expression's
animation frames are resident; when the active expression changes, the previous
animation's frames are dropped and the new one's are rasterized.

## Manifest Schema

Animations are declared under a new top-level `animations` key in `mod.json`, and
in an embedded default table (below). Each entry is keyed by expression name.

```jsonc
"animations": {
  "speaking": {
    "frames": ["speaking_0","speaking_1","speaking_2","speaking_3","speaking_4","speaking_5"],
    "driver": { "type": "amplitude", "curve": "sqrt" }
  },
  "thinking_dots": {
    "template": "dots.svg", "param": "V", "from": 0, "to": 3, "steps": 4,
    "driver": { "type": "time", "fps": 6, "mode": "loop" }
  }
}
```

**Frame source — exactly one of:**
- `frames`: an ordered list of SVG basenames (no extension), each resolved through
  the library (override → embedded). `steps = len(frames)`.
- `template` + `param` + `from` + `to` + `steps`: render the named Go-template SVG
  with `{{.<param>}}` set to `from + (to-from)*i/(steps-1)` for `i ∈ [0, steps)`.

**Driver** — `"amplitude"` string shorthand, or an object:
- `{ "type": "amplitude", "curve": "linear"|"sqrt", "idle": { "fps": N, "mode": "pingpong" } }`
  - `step = round(curve(signal) * (steps-1))`, `signal ∈ [0,1]` clamped.
  - `curve` defaults to `linear`; `sqrt` reproduces today's perceptual response.
  - `idle` is optional: when `signal ≤ 0`, fall back to a time oscillation so the
    animation keeps gently moving when no amplitude is available (preserves today's
    speaking behavior). Omitted → the animation simply holds step 0 at silence.
- `{ "type": "time", "fps": N, "mode": "loop"|"pingpong"|"once" }`
  - `loop`: `0,1,…,N-1,0,…` — stateless, from the absolute clock.
  - `pingpong`: `0,1,…,N-1,…,1,0,…` — stateless, from the absolute clock.
  - `once`: play `0 → N-1` a single time, then hold `N-1`, measured from the
    expression-start **epoch** (renderer-supplied).

**Tolerant parsing** (matching existing manifest behavior): a malformed or
incomplete entry is skipped with a log line and that expression falls back to its
static face. A missing `animations` key yields no mod animations (embedded defaults
still apply per the merge rule below).

## Signal Model

```go
// Signal yields the current normalized animation input in [0,1].
type Signal interface { Value() float32 }
```

v1 ships exactly one implementation: an amplitude signal fed from the existing
`VoicePipeline.CurrentAmplitude()` / `clips.Player.CurrentAmplitude()` (RMS, already
`[0,1]`). The **engine consumes a `float32` per tick** — the interface lives at the
wiring layer (`cmd/bmo-pak`), where the amplitude value is produced. A future
viseme or pitch source implements the same interface and is swapped in there, with
**no change to the engine or the manifest schema**.

## Engine API & Caching

```go
// AnimFrame returns the current animation frame for expr at w×h, or (nil,false)
// when expr has no animation (caller then uses the static Cache.Frame path).
//   clock  — absolute seconds (drives loop/pingpong)
//   epoch  — seconds since expr became the active expression (drives once)
//   signal — current amplitude in [0,1] (drives amplitude)
func (e *Engine) AnimFrame(expr string, w, h int, clock, epoch float64, signal float32) ([]uint32, bool)
```

- The engine resolves `expr` against its effective animation set. No match →
  `(nil, false)`.
- On a match, the active driver computes the step; the frame provider yields the
  SVG bytes for that step; the engine rasterizes (reusing `face.Rasterize`) and
  caches the result keyed by `(expr, step)` at the current `w×h`.
- **Scoped cache:** the engine keeps frames for only one expression. When the
  requested `expr` differs from the resident one (or `w×h` changes), it discards
  the old frames before serving the new animation. This bounds resident animation
  memory to a single animation regardless of how many a mod declares.
- The engine holds a `*Library` (for frame-list/template byte resolution) and is
  rebuilt on a live mod switch alongside the face `Cache`, so animations swap with
  no restart.

### Effective animation set (merge rule)

Mirrors the existing face override rules:

- **Embedded defaults** define the built-in animations (`speaking`).
- A **non-self-contained** (overlay) mod **inherits** the embedded defaults and
  **overrides/extends** them by name from its `mod.json` `animations` (mod wins).
- A **self-contained** mod does **not** inherit embedded animations — same rule as
  faces (`SelfContained()`). It declares its own; an undeclared expression renders
  as a static face.

## Renderer Integration (`internal/renderer/bmo.go`)

- `blitFace` loses the `if Canonical(expr) == ExprSpeaking { … }` block. The new
  flow: **try `Engine.AnimFrame`** → on a buffer, `copy` it into `r.pixels`; else
  fall back to the existing static `r.faces.Frame(expr,…)`; else `drawPlainFace`.
- The renderer tracks `lastExpr` and `exprStart` across `Draw` calls to compute the
  `epoch` (seconds since the current expression became active) for `once`.
- `clock` is the existing `phase`; `signal` is today's `FrameState.SpeakAmplitude`
  (retained; documented as the generic amplitude-signal value). `cmd/bmo-pak` keeps
  passing the same fields — no caller-facing change.
- The renderer holds the `*face.Engine` via a setter mirroring `SetFaces`
  (e.g. `SetAnimations`), installed at startup and on mod switch.

## Dogfood: `speaking` Through the Engine

1. A one-time generator (a small `go generate` target reusing today's `speakParams`
   / `renderSpeakSVG` geometry) emits `speaking_0.svg … speaking_5.svg` into
   `internal/face/assets/`; these are committed as ordinary assets.
2. The embedded default animation table declares:
   `speaking` → `frames: [speaking_0..speaking_5]`,
   `driver: { type: amplitude, curve: sqrt, idle: { fps, mode: pingpong } }`,
   with idle values chosen to approximate today's gentle silence oscillation.
3. **Deleted from the runtime:** `speakSet`, `Strip`, `Cache.Speak`, `warmSpeak`,
   `renderSpeakLevels`, `buildSpeakLocked`, `speakBand`, the renderer's `blitStrip`
   overlay path, and the speaking special-case. The geometry math survives only in
   the offline generator.

Net: `speaking` is rendered by the same engine as a modder's custom talk animation —
demonstrating the engine is general, not a reskin of the old special case.

## Testing

`face`/engine tests run under `CGO_ENABLED=0`; renderer tests under `CGO_ENABLED=1`.

- **Frame providers:** frame-list provider resolves `name_i.svg` through the
  library; template provider samples `param` across `[from,to]` over `steps` and
  renders the template (e.g. `V` at `0,1,2,3` for a 4-step `0→3`).
- **Drivers:**
  - amplitude: `step = round(curve(signal)*(steps-1))` for `linear` and `sqrt`,
    clamped at `0` and `steps-1`; `idle` engages only when `signal ≤ 0`.
  - time: `loop`, `pingpong`, and `once` index math from `(clock, epoch, fps)`;
    `once` holds the last frame past completion.
- **Manifest parsing:** valid `frames` form, valid `template` form, a malformed
  entry skipped with a log and falling back to static, missing `animations` key
  tolerated.
- **Merge rule:** overlay mod inherits + overrides embedded; self-contained mod
  does not inherit; mod entry wins by name.
- **Engine/cache:** `AnimFrame` returns a buffer for an animated expr and
  `(nil,false)` for a static one; the scoped cache holds only the active
  expression's frames (switching expression discards the previous frames); a
  resolution change re-rasterizes.
- **Dogfood fidelity:** the engine-driven `speaking` frames match the prior
  6-level output (extends the existing `internal/face/fidelity_test.go` approach);
  the renderer no longer references any speaking-specific symbol.
- **cmd/bmo-pak:** no unit tests (by design); verified by `CGO_ENABLED=0 go build`,
  the full suite, `golangci-lint run ./...`, and a manual mod-switch check that a
  mod-declared animation plays and reverts on switch-back.

## Out of Scope / Deferred

- **Richer audio signals** (visemes/phonemes, pitch) — `Signal` interface is ready;
  no implementation ships in this spec.
- **Inter-frame interpolation / easing** — the engine snaps to discrete steps.
- **Cross-expression transitions / blending** — each expression animates
  independently; switching is a hard cut.
- **Native SMIL SVG animation** — unsupported by oksvg on-device; out of scope.
- **Prompt-injection / asset-trust hardening** — mod animation files are trusted on
  the same model as existing mod faces and prompt files; not revisited here.
