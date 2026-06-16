# Design: Emotion-aware talking animation, face logging, and settings model cycling

**Date:** 2026-06-16
**Status:** Approved (brainstorm) — pending implementation plan

## Summary

Three independent workstreams, requested together:

1. **Emotion-aware talking animation** — fix the regression where BMO's mouth no
   longer moves while speaking, by making every emotion face animatable through
   the *same declarative animation engine that mods use* (dogfooding). The
   built-in faces become a living reference for modders.
2. **Active-face debug logging** — log which face/animation is being rendered,
   including whether it came from an embedded default or a mod override, and
   whether it is animated or static.
3. **Settings menu** — make the item list scrollable (fix overflow/footer
   overlap) and let the user swap AI models with LEFT/RIGHT by selecting among a
   *list of providers* configured per kind (STT/Chat/TTS).

Each workstream is its own section below and can become its own implementation
plan / PR.

---

## Background / current state

- The render loop in `cmd/bmo-pak/main.go` computes a single `expr` string per
  frame. During `StateSpeaking` it uses the LLM-directed emotion when present,
  else the literal `speaking` face:

  ```go
  case assistant.StateSpeaking:
      if snap.Emotion != "" { expr = string(snap.Emotion) }
      else                  { expr = string(assistant.ExpressionSpeaking) }
  ```

- The animation engine (`internal/face/anim_engine.go`) keeps **one** animation
  resident at a time and only animates expressions that have an `AnimationDef`.
  `DefaultAnimations()` defines an animation **only** for `speaking`
  (`speaking_0..5.svg`, amplitude-driven).
- Emotion faces (`happy`, `excited`, …) have **no** animation def, so
  `Engine.AnimFrame` returns `(nil,false)` and the renderer falls back to the
  **static** SVG (`Renderer.blitFace`, `internal/renderer/bmo.go`).

**Root cause of the regression:** the LLM-directed-emotion feature (shipped) now
sets `snap.Emotion` on essentially every spoken reply, so `expr` is almost always
a static emotion face during speech → the mouth never animates. The talking
animation only plays on the rare reply with no emotion tag.

### Face inventory finding (smile vs laugh)

- `happy.svg` — a *distinct* closed-curve smile (two dot eyes + a curved line).
  Unrelated to the duplication; keep.
- `smile.svg` — original face, refined 3×; open mouth built from the
  teeth/tongue rounded-rect construction the lip-sync `speaking_*.svg` frames
  descend from. The versatile one.
- `laugh.svg` — near-duplicate from the bulk 25-asset commit; mouth ~7px lower,
  eyes slightly higher. Redundant. **Drop it.**

### Idle cycling finding

`internal/assistant/idle.go` pools only draw from ~7 faces
(`blink, look_around, neutral, smile, whistle, laugh, sleeping`). The 25+ emotion
SVGs are reachable only during LLM speech. Widen the pools.

---

## Workstream 1 — Emotion-aware talking animation (engine-driven, dogfooded)

### Principle

Animate the built-in default faces **through the same engine and declarative
format mods use** — no bespoke renderer code path. A modder animating their own
emotion does exactly what we do for the built-ins; the built-ins are the
reference implementation.

### Mechanism

The engine already supports a `Template` animation source: one Go-template SVG
with a single numeric `param` interpolated across `[From, To]` over `Steps`,
selected each tick by a `Driver`. The `DriverAmplitude` driver with **no `Idle`**
returns frame 0 when `signal <= 0` (verified in `anim_driver.go`). Therefore:

- A templated emotion face animated by amplitude **sits at rest (frame 0) when
  silent** and **animates its mouth with voice amplitude while speaking**.

### Design

1. **Templated emotion SVGs.** Convert each emotion face in the core set into a
   Go-template SVG whose mouth opening is driven by a param `m ∈ [0,1]`:
   - `m = 0` → the emotion's natural resting mouth (identical to today's static
     look).
   - increasing `m` → progressively talking-open mouth.
   - Templates must render to the rest pose when executed with **no data**, so
     static/idle rasterization keeps working without special handling
     (e.g. `{{$m := or .m 0.0}}` then use `$m`). `internal/face/anim_frames.go`
     already executes templates; `internal/face/raster.go` / the cache path must
     execute the template with empty data before rasterizing when the SVG bytes
     contain `{{`.

2. **Built-in animation defs.** Add an `AnimationDef` per templated emotion in
   `internal/face/anim_defaults.go`: `Template{File, Param:"m", From:0, To:1,
   Steps:6}` + `Driver{Kind:DriverAmplitude, Curve:curveSqrt}` (no `Idle`). These
   use the exact `AnimationDef` struct that mod manifest JSON parses into.

3. **`speaking` becomes "neutral, talking".** Retire `speaking_0..5.svg` and the
   standalone `speaking` animation. In `main.go`, when in `StateSpeaking` with no
   emotion set, `expr = neutral` (instead of `speaking`); the templated `neutral`
   face then animates via its amplitude def. `ExprSpeaking`/`ExpressionSpeaking`
   constants are removed once no longer referenced.

4. **Regression fix is automatic.** No speaking-vs-emotion special-casing needed
   in `main.go`: during `StateSpeaking`, `expr` is already the emotion; once that
   emotion has a def, `blitFace` → `AnimFrame` animates it (eyes/brows from the
   emotion SVG, mouth from amplitude).

5. **Prewarm.** Extend the existing `Prewarm(speaking)` calls to prewarm the
   likely-next talking face(s) so the first animated frame is smooth. At minimum
   prewarm `neutral`.

6. **Drop `laugh`.** Delete `laugh.svg`, remove `ExprLaugh` from the vocabulary
   (`internal/face/expr.go`, `internal/assistant/state.go`), and replace its uses
   in `internal/assistant/idle.go`.

7. **Widen idle pools.** Update `idle.go` pools to cycle more of the core emotion
   set (e.g. add `content`, `excited`, `concerned`) for richer idle behaviour.

### Scope of this implementation

Engine/loader work is done once. Template + add defs for the **core set** now:

```
neutral, happy, smile, excited, content, concerned, sad, angry
```

Remaining ~17 emotions stay static (they still render fine — the engine falls
back to the static SVG when no def exists) and are templated in follow-up PRs.

### SVG authoring

Use the `bmo-face` skill for the mouth geometry; preview via `face.Rasterize`.
Mind the device oksvg arc-sweep gotcha (use Béziers, see project memory).

### Units & boundaries

- `internal/face` (engine, defs, templates, rasterization) — owns the animation.
- `cmd/bmo-pak/main.go` — only the `expr`/no-emotion wiring, kept minimal.
- `internal/assistant/idle.go` — idle pool composition.

### Testing

- Driver: amplitude 0 → frame 0; rising amplitude → higher frames (existing
  `anim_driver_test.go` patterns).
- Template render: each core emotion executes with empty data (rest) and with
  `m=1` (open) without error and rasterizes to the right size.
- Library/cache: a templated SVG rasterizes statically (rest pose) via the cache
  path.
- `idle.go`: `laugh` no longer appears; new emotions appear; no panics.
- Regression guard: with an emotion set and `Speaking=true`/amplitude>0,
  `blitFace`/`AnimFrame` returns animated frames (not the static face).

---

## Workstream 2 — Active-face debug logging

### Goal

Make it possible to see, in the logs, exactly which face/animation BMO is
rendering and where it came from — for both internal (embedded) and mod faces.

### Design

- Emit a `debug`-level log **only when the rendered expression changes** (avoid
  per-frame spam), from the render loop in `cmd/bmo-pak/main.go` after `expr` is
  finalized for the frame.
- The line includes:
  - the **expression name** (`expr`),
  - the **source**: `mod-override` vs `embedded-default` — the cache already
    tracks this in its `resolved` map (`internal/face/cache.go`); expose a small
    accessor (e.g. `Cache.Source(expr) string` or have `Library`/`Cache` report
    whether the resolved bytes came from disk override or embedded),
  - whether it is **animated** (engine has a ready def for it) or **static** —
    via `animEngine.Has(expr)` / `Ready(expr)`.
- Example: `face: rendering "happy" (embedded-default, animated)`.

### Units & boundaries

- `internal/face` exposes the source/animated facts (no logging there).
- `cmd/bmo-pak/main.go` owns the change-detection + log emission.

### Testing

- `internal/face`: source accessor returns `mod-override` when a mod override is
  present, `embedded-default` otherwise.
- Change-detection helper logs once per change, not per frame (unit-test the
  helper in isolation if extracted).

---

## Workstream 3 — Settings menu: scrolling + provider model cycling

### Data model (no backward compatibility)

Each kind becomes a set of providers with an explicit active selection. Introduce
a `ProviderSet` type in `internal/config/config.go`:

```go
type ProviderSet struct {
    Active    string     `json:"active"`    // name of the active provider
    Providers []Provider `json:"providers"`
}

func (s ProviderSet) Current() Provider   // active, or first, or zero
func (s *ProviderSet) Cycle(delta int)    // move Active to prev/next provider
func (s ProviderSet) Names() []string
```

`Config` fields change:

```go
STT  ProviderSet `json:"stt"`
Chat ProviderSet `json:"chat"`
TTS  ProviderSet `json:"tts"`
```

JSON shape:

```json
"chat": {
  "active": "openai-4o",
  "providers": [
    {"name": "openai-4o", "model": "gpt-4o", "base_url": "...", "api_key": "..."},
    {"name": "local-llama", "model": "llama3", "base_url": "...", "api_key": "..."}
  ]
}
```

- `Validate` (AI mode): each kind has ≥1 provider, `Active` names an existing
  provider, and `Current()` passes the existing `validateAIProvider` checks.
- `Normalize`: default `Active` to the first provider's name when empty.
- **No migration code.** Any existing `config.json` using the old single-object
  layout is updated **manually as a one-off** — including:
  - repo defaults / `config.Default()` (`internal/config/config.go`),
  - test fixtures (`internal/config/config_test.go`, `internal/assistant/voice_test.go`,
    `internal/ui/screen_setup_test.go`, `internal/ui/settings_menu_test.go`),
  - the device file `/mnt/SDCARD/.userdata/tg5040/BMO/config.json` (via ADB).

### Consumer updates

Replace `cfg.STT` / `cfg.Chat` / `cfg.TTS` reads with `cfg.STT.Current()` etc.
Touch points (from grep): `cmd/bmo-pak/main.go:230-233`,
`cmd/generate-audio/main.go:53-54`, `internal/ui/screen_settings.go`,
`internal/ui/screen_setup.go`, `internal/ui/settings_menu.go`.

### Settings UI — model cycling

- When `Mode == AI`, the `STT`/`CHAT`/`TTS` rows become **focusable** (today they
  are `Disabled`). `ToggleFocused` / a new LEFT/RIGHT handler cycles the active
  provider for the focused kind via `ProviderSet.Cycle(±1)`; the row label shows
  the active provider summary (reuse `providerSummaryLabel` /
  `providerModelLabel`). Auto-saved like the other cycles.
- When AI is off, the rows stay disabled as today.
- LEFT/RIGHT semantics align with existing cycles (MOD, timeout, proactive).
  Confirm `internal/input/nav.go` already surfaces LEFT/RIGHT to the menu (it was
  recently modified); wire the focused-kind cycle accordingly.

### Settings UI — scrolling viewport

`Renderer.drawOverlay` (`internal/renderer/bmo.go:517`) currently increments
`top` for every item with no bound, so items overflow the panel and overlap the
footer.

- Compute `maxVisibleRows` from the panel content height and the per-row stride.
- Track the focused item index (the `SettingsMenu` knows `m.focus`; surface it on
  `OverlayState`, e.g. `FocusIndex int`).
- Maintain a scroll offset so the focused row is always within the visible
  window; clip rows outside the window.
- Draw ▲ / ▼ affordances when content extends above/below the window.

### Units & boundaries

- `internal/config` — `ProviderSet` type + validation/normalization.
- `internal/ui/settings_menu.go` — focusability + cycle handling, `FocusIndex`.
- `internal/renderer/bmo.go` — viewport/scroll/clip in `drawOverlay`.

### Testing

- `ProviderSet`: `Current`/`Cycle`/`Names` incl. wrap-around and empty/zero sets;
  `Validate`/`Normalize` with active-name resolution; JSON round-trip of the new
  shape.
- `settings_menu_test.go`: LEFT/RIGHT cycles the active provider when AI on;
  rows non-focusable when AI off; label reflects active provider.
- Renderer: items beyond `maxVisibleRows` are clipped; focused item stays
  visible after scrolling; ▲/▼ indicators appear correctly. (CGO+SDL build.)

---

## Cross-cutting

- Build/test: `CGO_ENABLED=1 go test ./...`; `golangci-lint run ./...` after
  every change (project memory).
- TrimUI Smart Pro (tg5040) renders at **1024×768** (4:3) — matches the 280×210
  SVG viewBox aspect, so mouth-region geometry maps without distortion.
- Commits omit the `Co-Authored-By` trailer (project memory).
