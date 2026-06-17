# Whistle & Look-Around Idle Animations — Design

*Date: 2026-06-17*

## Problem

`ExpressionLookAround` (`"look_around"`) and `ExpressionWhistle` (`"whistle"`) are
declared in `internal/assistant/state.go` and used in **all three** idle schedules
in `internal/assistant/idle.go` (chatty/normal/quiet). They have no SVG asset, no
`face.Canonical` case, and no animation, so every time the idle scheduler picks
them BMO silently shows a frozen `neutral` face. The two "liveliness" beats in the
idle loop are therefore dead — they look identical to plain neutral.

This is recorded in the auto-memory `reference_whistle_lookaround_fold_to_neutral`,
which becomes obsolete once this work ships.

## Goal

Make `look_around` and `whistle` render as distinct, gently animated idle faces,
fitting the existing declarative animation engine, without exposing them to the LLM
emotion vocabulary (they are state/idle-driven, never model-requested).

## How the existing engine works (constraints)

- `internal/renderer/bmo.go` blits the **raw** expression name into
  `Engine.AnimFrame` (keyed by lowercased name). An animation registered under
  `"whistle"`/`"look_around"` plays without any `Canonical` change.
- `face.DefaultAnimations()` declares animations as either an explicit frame list
  (`speaking_0..5`) **or** a single templated SVG with one float `Param` swept
  `From→To` over `Steps` frames (every emotion). `animFuncs` provides `add/sub/mul`.
- Drivers: `DriverAmplitude` (voice lip-sync, used by emotions) and `DriverTime`
  (`FPS` + `Mode` ∈ `loop|pingpong|once`). Idle faces have no audio, so they use
  `DriverTime`.
- Static fallback: before an animation's frames finish building (or if the engine
  is unavailable), the renderer falls back to `faces.Frame(expr)`, which resolves
  through `face.Canonical` and renders the template at its **rest** value (param
  default, via `renderRestSVG`). So each template's `param=default` must read as a
  sensible still face.
- LLM emotion vocabulary = `CanonicalNames` − `FunctionalNames`
  (`face.EmotionNames`). `blink`/`sleeping` are already "functional" idle faces
  excluded from the model; `whistle`/`look_around` join them.
- `cache.go` warms every `CanonicalNames` entry as a static face at startup, so
  adding the names there gives a warm resting fallback.

## Design

### Assets (templated SVGs, 280×210 viewBox, Bézier curves only)

`internal/face/assets/look_around.svg`
- Param `x`, default `0.0` (`{{$x := or .x 0.0}}`).
- Eyes drawn as the neutral pair (`cy=78`, `r=6.5`) with horizontal offset:
  left eye `cx = 80 + x*14`, right eye `cx = 199 + x*14`. Both eyes move together
  (a glance), not converging.
- Mouth: the neutral resting smile path (static), no talkmouth.
- Background/bezel identical to `neutral.svg`.
- At rest (`x=0`) this is visually the neutral face → clean static fallback.

`internal/face/assets/whistle.svg`
- Param `t`, default `0.0` (`{{$t := or .t 0.0}}`).
- Eyes: neutral pair (static).
- Mouth: a small pursed "o" — a filled dark circle (~r 9) centered near the
  neutral mouth (≈ cx 140, cy 116) with a lighter inner fill for depth. Constant
  across the loop (recognizably "whistling").
- Music note: a single ♪ drawn from Béziers (note head ellipse + stem + flag),
  near the mouth, that rises and fades as `t` 0→1: `cy = 80 - t*40`,
  `fill-opacity = 1 - t` (clamped ≥ 0 via template arithmetic), slight rightward
  drift `cx = 168 + t*18`.
- At rest (`t=0`) the note sits low and fully visible by the mouth → acceptable
  still face; the loop makes it float up repeatedly.

Both faces must be previewed via `face.Rasterize` before finalizing (device oksvg
renders degenerate arc sweeps opposite to ImageMagick — use Béziers, per
`reference_oksvg_arc_sweep`).

### Engine registration (`anim_defaults.go`)

```go
ExprLookAround: {
    Template: &TemplateSource{File: ExprLookAround, Param: "x", From: -1, To: 1, Steps: 5},
    Driver:   Driver{Kind: DriverTime, FPS: 3, Mode: modePingpong},
},
ExprWhistle: {
    Template: &TemplateSource{File: ExprWhistle, Param: "t", From: 0, To: 1, Steps: 6},
    Driver:   Driver{Kind: DriverTime, FPS: 4, Mode: modeLoop},
},
```

- `look_around`: pingpong over `x ∈ {-1,-0.5,0,0.5,1}` at 3 fps → a full
  left↔right scan in ≈2.7s; 1–2 scans over the 3–6s idle dwell.
- `whistle`: loop over `t ∈ {0,…,1}` at 4 fps → note rises over ≈1.5s and repeats.

### Canonical / vocabulary wiring (`expr.go`, `emotion.go`)

- Add constants `ExprWhistle = "whistle"`, `ExprLookAround = "look_around"`.
- Add both to `CanonicalNames` (warms a real resting static fallback).
- Add both to `FunctionalNames` (excluded from `EmotionNames` → never advertised to
  the chat model). `EmotionFaceNamesInDir` already filters functional names, so mods
  won't expose them either.
- Add `Canonical` cases: `case ExprWhistle: return ExprWhistle`,
  `case ExprLookAround, "lookaround": return ExprLookAround`.

### No change required in

- `internal/assistant/idle.go` and `state.go` — they already reference the
  expressions; they simply start rendering correctly.

## Testing

Following TDD, before implementation:

- `expr_test.go`: `Canonical("whistle")=="whistle"`, `Canonical("look_around")==
  "look_around"`, `Canonical("lookaround")=="look_around"`.
- `emotion_test.go`: `EmotionNames()` excludes `whistle`/`look_around` (they are
  functional), but `CanonicalNames` includes them.
- `anim_defaults_test.go`: `DefaultAnimations()` has entries for both, each
  `DriverTime`, `Steps() >= 2`, valid template (param/file present).
- Asset/build coverage: `buildFrames` succeeds for both at a sample size (frames
  rasterize without error) — exercises the new SVG templates end-to-end.
- A driver assertion that `look_around` pingpong and `whistle` loop produce the
  expected frame index progression over time (reuse existing driver test style).

Full verification gate: `CGO_ENABLED=1 go test ./...` and
`golangci-lint run ./...` clean.

Optional (not blocking): regenerate the face gallery via `cmd/render-faces` and the
gallery doc, since two faces are added.

## Post-merge bookkeeping

Update auto-memory: `reference_whistle_lookaround_fold_to_neutral` is now wrong;
replace it with a note that whistle/look_around are time-driven idle animations
(`DefaultAnimations`) and functional (excluded from the LLM vocab).

## Non-goals

- No head/eye bob (kept minimal per design decision).
- No LLM-directed use of these faces.
- No new driver kinds or engine changes — reuse `DriverTime`.
