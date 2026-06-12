# SVG-Based Face Rendering — Design

**Date:** 2026-06-12
**Status:** Approved for planning

## Problem

BMO's face is drawn by a hand-written software rasterizer (`internal/renderer/bmo.go`).
Thick curves (smile, brows, arc eyes) are produced by stamping filled circles along
bezier points (`drawBezierThick`), and nothing is anti-aliased. On the TrimUI Brick
(1024×768) this passes, but on the Smart Pro (1280×720) and any larger screen the
construction artifacts are clearly visible: lumpy mouth edges, stepped eyebrows.

## Goals

1. Smooth, anti-aliased face rendering at any resolution.
2. Expressions defined as SVG files — verifiable through code and math, and editable
   by humans and Claude alike (the bmo-face skill already speaks this language).
3. Moddable: users replace SVG files in a `faces/` folder on disk (plus persona and
   voice) — no compilation.
4. Keep the amplitude-driven speaking mouth animation for the default pack.
5. No new CGO/C dependencies; the tg5040/tg5050 container cross-build stays untouched.

## Non-Goals

- Animated mouths for mod-provided speaking faces (future extension).
- Full SVG spec support; a documented subset is sufficient.
- Changing the dynamic overlays (settings menu, quota clock, ZZZ marks, backdrop
  sparkles) — these stay procedural.

## Decisions Made

| Question | Decision |
|----------|----------|
| SVG library | `srwiley/oksvg` + `srwiley/rasterx` (pure Go, anti-aliased) |
| Speaking mouth | Quantized: Go-templated SVG rendered at 12 openness levels |
| SVG file contents | Full scene (body + screen + face), 280×210 viewBox |
| 16:9 vs 4:3 | Non-uniform stretch to fill the screen (matches current behavior) |
| Asset location | `faces/` on disk in the pak; `go:embed` copies as fallback |
| Mod validation | Parse-or-fallback only; geometry tests are internal CI QA |

## Architecture

New package `internal/face`:

- **`face.Library`** — resolves an expression name to SVG bytes.
  Lookup order: `faces/<name>.svg` on disk → `faces/<canonical>.svg` on disk
  (via the existing `normalizeExpression` aliasing, e.g. `laugh` → `smile`) →
  embedded default. Per-file fallback: a mod may override a single expression.
- **`face.Cache`** — rasterizes a parsed SVG into an ARGB8888 pixel buffer at the
  current output resolution and caches it per `(expression, W, H)`. A resolution
  change (renderer `SyncSize`) invalidates the cache.
- **Warm-up** — the current expression is rasterized synchronously at startup; the
  remaining expressions and speaking levels pre-warm in a background goroutine.
  A cache miss at draw time rasterizes on demand (one-time, ~tens of ms).

Renderer (`internal/renderer`) keeps the single-SDL-streaming-texture pipeline.
`Draw()` becomes:

1. Blit the cached face buffer into `r.pixels`.
2. Draw dynamic extras procedurally on top (unchanged): backdrop sparkles, ZZZ sleep
   marks, quota clock, settings overlay, bitmap text.
3. Upload texture, present.

The procedural face code (`drawFace`, eye/brow/mouth drawing, `drawMouthFilled`,
`drawBezierThick`, and friends) is deleted. `FrameState` and the main loop are
untouched.

## Asset Format & Modding Contract

```
faces/
  neutral.svg     blink.svg      listening.svg   thinking.svg
  speaking.svg    sleeping.svg   concerned.svg   smile.svg
  README.md
```

- Full scene per file: teal body `#4ECBA8`, screen interior `#90e5c8`, face elements —
  exactly the bmo-face skill's 280×210 boilerplate. WYSIWYG in any SVG editor.
- Default assets derive from the bmo-face skill element library. The two clip-path
  mouth constructions are rewritten as explicit paths (teeth band = path with rounded
  top corners; tongue = upper half-ellipse, which never crosses the mouth's rounded
  corners at canonical sizes, so the clip was a no-op). Same visible geometry.
- Supported SVG subset (documented in `faces/README.md` for modders): `path` (all
  commands), `rect`, `circle`, `ellipse`, `line`, `polygon`, `polyline`, `g`,
  `defs`/`use`, transforms, fill/stroke/opacity, linear/radial gradients.
  **Not supported:** `clipPath`, filters, masks, text, CSS classes, `pattern`.
- Scaling: the viewBox is stretched non-uniformly to fill the full output. On 16:9
  BMO is slightly wider than the 4:3 reference; no letterboxing.

## Speaking Mouth (Default-Pack Special Case)

- The shipped `speaking.svg` is a Go text template with one parameter: mouth
  openness ∈ [0,1], controlling the mouth-band height within the canonical
  y=101–144 / x=81–165 region.
- At warm-up it is executed at 12 levels. Level 0 renders the full base frame; for
  levels 1–11 only the mouth-band rectangle is retained as a strip (~0.5 MB each at
  720p) to bound memory.
- Per frame, `SpeakAmplitude` maps through the existing sqrt curve to the nearest
  level; the strip is blitted over the base.

**Self-detection:** at load time, `speaking.svg` is inspected for template action
markers (`{{`):

1. Markers present → execute at all 12 levels; all parse as valid SVG → animated.
2. No markers, or execution/parse fails → treat as a plain **static** SVG during
   speech (logged as informational — this is the expected mod path).
3. Static parse also fails → embedded default template.

## Error Handling

- Mod file missing → embedded default (silent, per-file).
- Mod file fails to parse/rasterize → warning log naming the file; embedded default.
  BMO never shows a broken face because of a bad mod file.
- Embedded defaults are guaranteed by CI tests; as a last-resort runtime guard the
  renderer falls back to plain body+screen rounded rects rather than crashing.
- Memory budget: ~8 full frames + speaking base + 11 strips ≈ 35–40 MB at 1280×720.

## Testing (internal QA — mod files are never held to these)

- Every shipped/embedded asset parses and rasterizes at 1024×768 and 1280×720,
  producing non-blank output.
- Geometry assertions from the bmo-face proportions table: sampled pixels at eye
  centers (20.3% / 79.2% width, 37.4% height of screen interior) are dark
  `#1a1a1a`; screen center is `#90e5c8`; corners are body teal `#4ECBA8`.
- Speaking monotonicity: dark-pixel count in the mouth band increases with level.
- Fallback: corrupt SVG in a temp `faces/` dir logs and falls back to default.
- Template detection: static `speaking.svg` is accepted and used statically.
- `golangci-lint run ./...` clean; `internal/renderer/bmo_test.go` adapted.

## Follow-Ups (out of scope)

- Update the bmo-face skill's mouth library to the clipPath-free idiom so future
  mockups are copy-paste-ready for the renderer.
- Optional: animated-mouth support for mod packs (documented template contract).
- Funny-voice easter egg setting (tracked separately in project memory).

## Dependencies Added

- `github.com/srwiley/oksvg` (pure Go)
- `github.com/srwiley/rasterx` (pure Go)
- transitively: `golang.org/x/image`, `golang.org/x/net` (charset)
