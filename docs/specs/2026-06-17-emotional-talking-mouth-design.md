# Emotional Talking Mouth — Design

**Status:** Approved
**Date:** 2026-06-17

## Goal

Make BMO's talking animation read as talking for every emotion. Today only the
dedicated `speaking` face shows a real open mouth; emotion faces animate their
own resting mouth shape, which looks wrong while speaking. Unify the model so
that **every animatable emotion keeps its eyes/brows and natural resting mouth
at silence, and opens a shared teeth/tongue mouth while audio plays.** Also fix
the mouth starting to move late on the startup "hello" clip.

## Background — the three observed issues

1. **Hello mouth starts late.** The speaking animation is amplitude-driven
   (`anim_driver.go`, `DriverAmplitude`, `sqrt` curve, ping-pong idle only when
   `signal <= 0`). As the hello clip ramps up from quiet, low amplitude maps to
   a near-closed frame, so the mouth stays shut until the audio gets loud.
2. **Excited "double mouth."** `excited.svg` keeps its static closed-lip
   teeth/tongue art **and** draws a separate `$m`-driven dark gap on top. The
   teeth/tongue shape never moves; a dark blob grows over it — two mouths.
3. **Angry wiggling closed mouth.** `angry.svg`'s mouth is one frown *line*
   whose control point slides down with `$m`. It never opens.

Issues 2 and 3 share a root cause: WS1 animates each emotion's own mouth shape,
which does not read as talking. Issue 1 shares a mechanism with emotional
talking — quiet audio under-opens the mouth.

## Design

### Core model (all 8 animatable emotions)

The animatable set is `neutral, happy, smile, excited, content, concerned, sad,
angry`. Each emotion SVG keeps its emotion-specific eyes/brows/cheeks (static,
unchanged). The mouth becomes conditional on the amplitude template param `$m`:

- `$m == 0` (silence / rest) → the emotion's **natural mouth** (today's resting
  art).
- `$m > 0` (talking) → the **shared open teeth/tongue mouth**, opened
  proportionally to `$m`.

The animation driver is unchanged: `DriverAmplitude`, no idle, 6 steps. With no
idle, the driver rests at frame 0 (m=0) during silence and steps toward frame 5
(m=1) as voice amplitude rises. Because `$m == 0` art is identical to today's
resting art, rest-fidelity and the embedded-geometry tests are preserved.

The natural→open transition at speech onset is a discrete swap (frame 0 →
frame 1+), matching how the dedicated `speaking` face already pops from closed
to open. This is the intended behavior: natural mouth when idle, open mouth when
talking.

### The shared open mouth

A single open teeth/tongue mouth — the `speaking` face look: a teeth band on
top, tongue at the bottom, dark interior — parameterized so its opening grows
with `$m`, positioned in the common mouth region every BMO face uses
(roughly `x ∈ [98, 182]`, `y ∈ [101, 145]`). The shape is identical across all
emotions; that consistency is what makes emotional talking look uniform.

It is built from the known-good `speaking_*` frame geometry (which renders
correctly on-device). Each emotion template is verified to rasterize cleanly at
rest (`$m = 0`) and fully open (`$m = 1`) via `face.Rasterize`, guarding against
the device oksvg arc-sweep gotcha (degenerate sweeps render opposite to
ImageMagick). Because `$m > 0` always means "at least slightly open," the open
shape is never degenerate.

The open-mouth markup is duplicated across the 8 emotion SVGs (they are embedded
data files). Duplication is acceptable; a generator is out of scope (YAGNI).

### Responsiveness fix (issue 1)

The goal: **the mouth opens promptly when audio starts and keeps moving while
audio plays, including quiet passages**, while preserving the amplitude-driven
lip-sync feel the user likes.

Mechanism is chosen by measurement, not assumption. Add a short instrumented
check (a debug log of clip amplitude over the first ~1s of the hello clip, like
the framebuffer probe), then pick the minimal matching fix:

- **(a) Low-amplitude mapping** — if quiet speech maps to frame 0, apply a small
  gain or a low floor to the signal during speech so onset opens the mouth.
- **(b) Amplitude latency** — if `CurrentAmplitude` lags the audio, align the
  sampling so the mouth tracks the sound.

The instrumentation is removed once the cause is confirmed and the fix landed.

## Units & boundaries

| File | Change |
|------|--------|
| `internal/face/assets/{neutral,happy,smile,excited,content,concerned,sad,angry}.svg` | Rewrite mouth as `{{if eq $m 0.0}}natural{{else}}shared open teeth/tongue mouth scaled by $m{{end}}`; eyes/brows unchanged. |
| `internal/face/anim_templates_test.go` | Extend: open frame (m=1) contains teeth `#e4e4e4` + tongue `#1a7848` for every emotion; rest frame (m=0) matches today's natural mouth. |
| `internal/face/anim_defaults.go` / driver | Unchanged for the core model. Touched only if the issue-1 fix needs a gain/floor knob. |
| `cmd/bmo-pak/main.go` / `internal/clips` or `internal/assistant` | Issue-1 instrumentation, then the minimal responsiveness fix at the confirmed layer. |

No engine or routing changes for the core model — it is entirely in the SVG
templates plus the existing amplitude driver.

## Testing

- **Per-emotion rest vs open:** for each of the 8 emotions, the rest frame
  (`renderRestSVG`, m=0) reproduces today's natural mouth, and the open frame
  (`renderAnimTemplate m=1`) contains the teeth (`#e4e4e4`) and tongue
  (`#1a7848`) colors of the shared open mouth. Line-mouth emotions (neutral,
  sad, concerned, angry, content, happy) must NOT contain teeth at rest, proving
  the natural mouth shows at silence rather than the open mouth.
- **Keep** existing `TestCoreTemplatesRenderRestAndOpen` (rest ≠ open) and
  `TestSpeakingEmotionAnimates` (regression guard).
- **Rest-fidelity / geometry:** `TestEmbeddedFacesGeometry` and the fidelity
  manifest still pass because m=0 art is unchanged.
- **Issue 1:** a focused test for whatever responsiveness fix lands (e.g. that a
  low-but-present amplitude yields a non-zero frame, or that sampling aligns).
- Full suite green under `CGO_ENABLED=1 go test ./...`, race-clean on
  `internal/face`, lint clean.

## Out of scope

- Functional faces (blink, listening, thinking, sleeping) — unchanged.
- The non-animatable Figma emotions (love, shy, crying, etc.) stay static.
- Cross-fading or sub-frame interpolation of the natural→open transition.
- A generator for the shared mouth snippet.
