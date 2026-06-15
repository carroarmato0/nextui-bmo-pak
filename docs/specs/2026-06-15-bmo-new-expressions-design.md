# BMO New Expressions — Design Spec

**Date:** 2026-06-15
**Status:** Approved (visual review complete)

## Goal

Expand BMO's expression catalog from 8 to 33 by adding 25 new face assets derived
from the community Figma "BMO Face Templates" file. This first pass **adds and
names** the expressions and wires them into the face-rendering pipeline so BMO
*can* show them. Deciding *when* BMO shows each one (assistant state machine, idle
animations, LLM advertising) is explicit follow-up work.

## Source

- Figma file: `do2FepNXsyNjidQSPM6VjU` ("BMO Face Templates & Assets (Community)").
- 30 frames (`Rosto-01`…`Rosto-30`) pulled as SVG via the Figma REST API.
- Frames are 1280×720 with a `#C9E4C3` background and black features in the
  community palette. We keep **our** palette, not theirs.

## Approach

### Coordinate mapping

Each Figma frame maps onto our existing canonical layout (matching the live
assets, **not** the older bmo-face skill doc) with a single affine transform
derived from eye positions:

```
s = 119/612 ≈ 0.1944          # our eye separation / Figma eye separation
x_our = 140 + (x_fig − 640)·s
y_our =  78 + (y_fig − 271)·s
```

Verified: this reproduces our existing `neutral.svg` (eyes cx≈80/199 cy=78,
mouth `M116 111 Q140 125 160 111`) from `Rosto-01`. The transform is used as a
*placement guide*; each face is then redrawn in our flat element-library style
(per the bmo-face skill: flat fills/strokes only, no gradients/blur/shadows),
because the device renderer is oksvg and many Figma frames use blur, inner
shadow, and blend modes that oksvg cannot render.

### Canonical layout (matches live assets)

- Viewport `280×210`; body rect teal `#4ECBA8`; screen rect `x=12 y=10 w=256
  h=188 rx=12` fill `#90e5c8`.
- Eyes: left `cx=80`, right `cx=199`, `cy=78`. Mouth centered at `cx=140`.

### Palette

Existing: body `#4ECBA8`, screen `#90e5c8`, features `#1a1a1a`, teeth `#e4e4e4`,
mouth interior `#1a7848`, tongue `#16ae81`.

**New colors (approved):**

| Role | Hex | Used by |
|------|-----|---------|
| Heart / tongue red | `#e8443b` | `love`, `playful` |
| Star / sparkle gold | `#f4c531` | `excited`, `sparkle` |
| Tears blue | `#5bc8e8` | `crying`, `teary`, `gloomy` |
| Blush green | `#53AF66` @ 0.55 | `shy`, `adoring` |

### New element primitives

Beyond the existing library (dot/pill/arc/flat eyes; smile/frown/open/speaking
mouths): big round eye + shine, 5-point star eye, red heart eye, spiral eye,
x-eye, half-lidded eye, dash eye, 4-point sparkle eye, `>`/`<` eyes; tear drops,
clenched-teeth mouth, tongue-out mouth, `3`-shaped (kiss) mouth, round "gasp"
mouth, wavy mouth, blush ellipses, 8-bit pixel face; angry / worried-sad /
single-raised brows. All render cleanly through `face.Rasterize` (oksvg) at both
1024×768 and 1280×720 — verified, no degenerate-arc issues (curves use
quadratics/lines, not problematic arc sweeps).

## The 25 new expressions

**Tier A — core emotions (9):**

| Name | Source | Composition |
|------|--------|-------------|
| `sad` | Rosto-03 | dot eyes + frown |
| `happy` | Rosto-18 | dot eyes + wide grin |
| `laugh` | Rosto-17 | squint arc eyes + open mouth |
| `content` | Rosto-04 | calm closed (∪) eyes + soft smile |
| `angry` | Rosto-10 | angry brows + dot eyes + frown |
| `surprised` | Rosto-22 | wide round eyes + small "o" mouth |
| `excited` | Rosto-20 | gold star eyes + open mouth |
| `love` | Rosto-21 | red heart eyes + smile |
| `shy` | Rosto-05 | dot eyes + blush + wavy mouth |

**Tier B — expressive extras (11):**

| Name | Source | Composition |
|------|--------|-------------|
| `crying` | Rosto-12 | closed eyes + blue tear streams + wail mouth |
| `teary` | Rosto-06 | worried-sad brows + big welling eyes + tears |
| `gloomy` | Rosto-11 | downcast eyes + sweat drop + frown |
| `dizzy` | Rosto-24 | spiral eyes + woozy wavy mouth |
| `unamused` | Rosto-16 | half-lidded eyes + flat mouth |
| `annoyed` | Rosto-29 | dash eyes + dash mouth (`-_-`) |
| `skeptical` | Rosto-09 | single raised brow + half-lid + off-center frown |
| `playful` | Rosto-14 | wink + tongue out |
| `kiss` | Rosto-19 | `>` `<` eyes + `3` mouth |
| `grimace` | Rosto-08 | dash eyes + clenched teeth |
| `shout` | Rosto-07 | angry brows + dot eyes + big open mouth |

**Tier C — stylized / emoticon novelties (5):**

| Name | Source | Composition |
|------|--------|-------------|
| `dead` | Rosto-25 | `x_x` eyes + flat mouth |
| `glitch` | Rosto-26 | 8-bit pixel eyes + pixel smile |
| `dismayed` | Rosto-27 | worried-sad brows + wide eyes + gasp mouth (`D:`) |
| `adoring` | Rosto-13 | shiny eyes + blush + sparkle accents + smile |
| `sparkle` | Rosto-23 | gold 4-point sparkle eyes + smile |

Skipped as duplicates of existing assets: `Rosto-01` (neutral), `Rosto-02`
(≈ speaking), `Rosto-15`/`Rosto-28`/`Rosto-30` (≈ smile/neutral).

## Wiring changes

1. **Assets:** add 25 `internal/face/assets/<name>.svg` (auto-embedded by the
   existing `//go:embed assets/*.svg`).
2. **`internal/face/expr.go`:**
   - Add 25 `Expr<Name>` constants.
   - Append all 25 to `CanonicalNames` (drives cache warm-up).
   - Extend `Canonical()` so each new name maps to itself, plus a small set of
     obvious aliases.
   - **Behavior change:** `happy`/`laugh`/`excited` no longer alias to `smile`;
     `sad`/`angry` no longer alias to `concerned` — each now resolves to its own
     asset. System-state aliases keep their meaning: `error`, `confused` →
     `concerned`; `idle` → `neutral`.
3. **Tests:**
   - `internal/face/expr_test.go`: update the reassigned alias expectations and
     add cases for the new names.
   - `internal/face/assets_test.go`: add at least eye-position geometry
     assertions for each new expression (rasterized at 1024×768 and 1280×720),
     following the existing sampling pattern.
   - **Fidelity test** (`internal/face/fidelity_test.go`): guarantees the
     shipped faces are exactly the artifacts approved in the browser previews.
     - The 25 approved candidate SVGs (the exact bytes the preview rendered) are
       frozen as a committed baseline manifest
       `internal/face/testdata/approved_expressions.json`, mapping each new
       expression name → `sha256` of its approved SVG bytes.
     - The test asserts, for every new expression, that
       `sha256(defaultBytes(name))` equals the frozen hash — so any later edit
       to an approved asset (or a regenerated/divergent file) fails loudly with
       "no longer matches the approved preview".
     - It also asserts each embedded asset rasterizes non-blank through
       `face.Rasterize` at 1024×768 and 1280×720 (the device path, oksvg).
     - Byte-identity is chosen over a golden *render* hash so the check is
       deterministic across machines/Go/oksvg versions; because the browser
       preview rendered these exact SVG bytes, byte-fidelity to them is fidelity
       to what was approved.
4. **Docs:** extend the `docs/FACES.md` face catalog table with the 25 new files.

## Out of scope (follow-ups — "later do more with them")

- Assistant-layer `assistant.Expression` constants in
  `internal/assistant/state.go` and any state-machine transitions that pick new
  faces.
- Idle-animation selection (`internal/assistant/idle.go`).
- Advertising the new expression vocabulary to the LLM in the persona/system
  prompt so the model can request them.
- Settings UI / per-expression overrides documentation beyond the catalog table.
- Updating the `~/.claude/skills/bmo-face` skill with the new primitives and
  palette (developer tooling, not shipped code).

## Verification

- `CGO_ENABLED=0 go test ./...` green (incl. new geometry + fidelity tests).
- `golangci-lint run ./...` adds no findings.
- The fidelity test confirms every shipped asset is byte-identical to the
  browser-approved baseline and rasterizes non-blank at 1024×768 and 1280×720.
