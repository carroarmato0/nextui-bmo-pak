# Talking Animation (Engine-Driven) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the regression where BMO's mouth no longer moves while speaking, by making each emotion in a CORE SET (`neutral, happy, smile, excited, content, concerned, sad, angry`) animatable through the SAME declarative animation engine that mods use. Built-in faces become the modder reference (dogfooding). The standalone six-frame `speaking_*` animation is retired; `speaking` state renders the templated, amplitude-driven `neutral` face. The `laugh` emotion is dropped from the vocabulary. Idle pools are widened to cycle more core emotions. Remaining ~17 emotions stay static (engine already falls back to static SVG when no animation def exists).

**Architecture:** The face package already has a complete declarative animation engine: `AnimationDef` (`anim_def.go`) describes either explicit `Frames` or a parametric `TemplateSource` driven by a `Driver`. `buildFrames` (`anim_frames.go`) renders each step — for a `Template` it calls `renderAnimTemplate(data, param, val)` (a Go `text/template` execution) then `Rasterize`. `Driver.Step` (`anim_driver.go`) for `DriverAmplitude` with **no** `Idle` returns frame 0 at silence (`signal <= 0`) and a higher frame as the voice signal rises (`sqrt` curve maps `v → sqrt(v)`); so an amplitude def with no `Idle` rests at silence and animates with voice. The engine (`anim_engine.go`) keeps one animation resident, builds async, and `blitFace` (`internal/renderer`) calls `Engine.AnimFrame` first, falling back to `Cache.Frame` (static). `buildAnimationEngine` in `cmd/bmo-pak/main.go` merges `face.DefaultAnimations()` with mod-parsed animations, so adding core-set defs to `DefaultAnimations()` automatically animates the built-ins and lets overlay mods inherit them.

The one structural change required is that the **static cache path** (`Cache.renderLocked`, `Cache.warmFrame`) and the **pixel-fidelity tests** rasterize raw SVG bytes directly. Once a core-set asset contains Go-template syntax (`{{ }}`), raw rasterization fails (`oksvg` cannot parse `{{...}}`). A new `renderRestSVG(data []byte) []byte` helper executes the template at rest (empty data → `or .m 0.0` yields 0) before rasterizing, leaving non-template SVGs untouched. Every core-set template renders BMO's exact current resting face when `m=0`.

**Tech Stack:** Go, oksvg/rasterx SVG rasterization, text/template, SDL2 (CGO).

---

## File Structure

| File | Create/Modify | Responsibility |
| --- | --- | --- |
| `internal/face/anim_frames.go` | Modify | Add `renderRestSVG(data []byte) []byte` helper that, when bytes contain `{{`, parses + executes the template with empty data (rest) and returns the rendered bytes; otherwise returns the input unchanged. |
| `internal/face/anim_frames_test.go` | Create | Unit-tests `renderRestSVG`: template bytes render at rest (no `{{` left, parses), plain SVG passes through byte-identical. |
| `internal/face/cache.go` | Modify | Route raw SVG bytes through `renderRestSVG` in `renderLocked` and `warmFrame` before `Rasterize`, so templated core-set faces rasterize at rest for static display. Remove the `ExprSpeaking` skip in `Warm`. |
| `internal/face/cache_test.go` | Create/Modify | Test that `Cache.Frame` returns a non-nil buffer for a templated core-set expression (e.g. `neutral`). |
| `internal/face/assets/neutral.svg` | Modify | Convert mouth to `m`-driven Go template (`m=0` = today's resting mouth). |
| `internal/face/assets/happy.svg` | Modify | Same — `m`-driven mouth template. |
| `internal/face/assets/smile.svg` | Modify | Same. |
| `internal/face/assets/excited.svg` | Modify | Same. |
| `internal/face/assets/content.svg` | Modify | Same. |
| `internal/face/assets/concerned.svg` | Modify | Same. |
| `internal/face/assets/sad.svg` | Modify | Same. |
| `internal/face/assets/angry.svg` | Modify | Same. |
| `internal/face/anim_templates_test.go` | Create | Per-core-asset test: each template rasterizes at rest (no data) and at `m=1`, and rest ≠ open. |
| `internal/face/anim_defaults.go` | Modify | Replace the single `speaking` def with eight core-set `Template` amplitude defs (no `Idle`); remove `speaking`. |
| `internal/face/anim_defaults_test.go` | Modify | Replace `TestDefaultSpeakingAnimation` / `TestDefaultSpeakingFramesRasterizeAndDiffer` with core-set equivalents. |
| `internal/face/assets_test.go` | Modify | Route core-set raw bytes through `renderRestSVG` before `Rasterize`; drop the `ExprLaugh` case. |
| `internal/face/fidelity_test.go` | Modify | Remove `ExprLaugh` from `newExpressions`; drop the five core-set emotions (`happy, content, angry, sad, excited`) from byte-fidelity guarding (they are now templates, not frozen art). |
| `internal/face/testdata/approved_expressions.json` | Modify | Remove the six no-longer-byte-frozen entries (`laugh` + the five core emotions). |
| `internal/face/expr.go` | Modify | Remove `ExprLaugh` (const, `CanonicalNames`, `Canonical` case). Keep `ExprSpeaking`. |
| `internal/face/emotion.go` | Modify | (No change required — `ExprSpeaking` stays functional; `laugh` was never functional.) Verify only. |
| `internal/assistant/state.go` | Modify | Remove `ExpressionLaugh`. Add `ExpressionExcited`/`ExpressionContent` if absent (verify — see Task 6). Keep `ExpressionSpeaking` as an assistant state value. |
| `internal/assistant/idle.go` | Modify | Widen `poolFor` pools to include `content`, `excited`, `concerned`, `happy`; replace all `ExpressionLaugh` uses. |
| `internal/assistant/idle_test.go` | Modify | Replace `ExpressionLaugh` references with new pool members. |
| `cmd/bmo-pak/main.go` | Modify | StateSpeaking no-emotion path → `ExpressionNeutral`; clipPlaying line 639 → `ExpressionNeutral`; Prewarm (291, 317) and Ready gate (539) → `face.ExprNeutral`. |
| `internal/face/assets/speaking_0.svg … speaking_5.svg`, `speaking.svg` | Delete | Retire the standalone speaking frames (done last, after defs no longer reference them). |

> **Decision recorded (spec WS1 item 3 + 4):** `ExprSpeaking` / `ExpressionSpeaking` are **kept** as semantic values. The assistant state machine still sets `m.expression = ExpressionSpeaking` (`internal/assistant/state.go:276`) when entering the speaking state — that is a state label, not a face request. Rendering maps the *speaking state* to the `neutral` face (which now animates via its amplitude def). Removing the constants entirely would force touching the state machine and every reference for no functional gain; the spec says remove "once no longer referenced," and they remain referenced as a state label. `laugh`, by contrast, has no remaining purpose and is removed completely.

---

## Task 1 — Add `renderRestSVG` rest-execution helper

**Files:**
- Modify `internal/face/anim_frames.go`
- Create `internal/face/anim_frames_test.go`

- [ ] Write a failing test `internal/face/anim_frames_test.go`:
```go
package face

import (
	"bytes"
	"testing"
)

func TestRenderRestSVGExecutesTemplate(t *testing.T) {
	in := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">` +
		`{{$m := or .m 0.0}}<rect x="0" y="{{$m}}" width="10" height="10"/></svg>`)
	out := renderRestSVG(in)
	if bytes.Contains(out, []byte("{{")) {
		t.Fatalf("template syntax left in output: %s", out)
	}
	// At rest, m=0 → the y attribute renders as "0".
	if !bytes.Contains(out, []byte(`y="0"`)) {
		t.Fatalf("rest value not 0: %s", out)
	}
	if _, err := Rasterize(out, 80, 60); err != nil {
		t.Fatalf("rest SVG must rasterize: %v", err)
	}
}

func TestRenderRestSVGPassesThroughPlain(t *testing.T) {
	in := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210"><rect/></svg>`)
	out := renderRestSVG(in)
	if !bytes.Equal(in, out) {
		t.Fatalf("plain SVG should pass through unchanged")
	}
}
```
- [ ] Run it (expect FAIL — `renderRestSVG` undefined): `CGO_ENABLED=1 go test ./internal/face/ -run TestRenderRestSVG -v`
- [ ] Implement in `internal/face/anim_frames.go` (append after `renderAnimTemplate`):
```go
// renderRestSVG executes a Go-template SVG at rest (no parameter data) so a
// templated core-set face can be rasterized for static display. Templates use
// `{{$m := or .m 0.0}}`, so empty data yields the resting mouth. Bytes that
// contain no template syntax are returned unchanged. On any parse/execute
// error the input is returned verbatim so Rasterize can surface the failure.
func renderRestSVG(data []byte) []byte {
	if !bytes.Contains(data, []byte("{{")) {
		return data
	}
	tmpl, err := template.New("rest").Parse(string(data))
	if err != nil {
		return data
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]any{}); err != nil {
		return data
	}
	return buf.Bytes()
}
```
- [ ] Run it (expect PASS): `CGO_ENABLED=1 go test ./internal/face/ -run TestRenderRestSVG -v`
- [ ] Lint: `golangci-lint run ./...`
- [ ] Commit: `git commit -am "face: add renderRestSVG to rasterize template faces at rest"`

---

## Task 2 — Route the static cache path through `renderRestSVG`

**Files:**
- Modify `internal/face/cache.go` (`renderLocked` line ~129, `warmFrame` line ~54, `Warm` skip line ~33)
- Create `internal/face/cache_test.go`

- [ ] Write a failing test `internal/face/cache_test.go`. This test depends on Task 3's templated `neutral.svg`; until then it guards behaviour against the embedded default. Write it now so it stays green through the asset conversion:
```go
package face

import "testing"

func TestCacheFrameRendersTemplatedNeutral(t *testing.T) {
	lib := NewLibrary(t.TempDir()) // embedded defaults only
	c := NewCache(lib)
	buf := c.Frame(ExprNeutral, 80, 60)
	if buf == nil {
		t.Fatal("neutral frame nil — templated face failed to rasterize at rest")
	}
	if len(buf) != 80*60 {
		t.Fatalf("frame size=%d want %d", len(buf), 80*60)
	}
}
```
- [ ] Run it (expect PASS today; it will keep guarding once `neutral.svg` becomes a template): `CGO_ENABLED=1 go test ./internal/face/ -run TestCacheFrameRendersTemplatedNeutral -v`
- [ ] In `internal/face/cache.go` `renderLocked`, wrap both `Rasterize` calls with `renderRestSVG`:
```go
func (c *Cache) renderLocked(canonical string, w, h int) []uint32 {
	data, fromDisk := c.lib.Bytes(canonical)
	if data == nil {
		return nil
	}
	buf, err := Rasterize(renderRestSVG(data), w, h)
	if err == nil {
		return buf
	}
	if fromDisk {
		c.lib.logf("face: override %s.svg failed to rasterize (%v); using default", canonical, err)
		if def, ok := defaultBytes(canonical); ok {
			buf, err = Rasterize(renderRestSVG(def), w, h)
			if err == nil {
				return buf
			}
		}
	}
	return nil
}
```
- [ ] In `internal/face/cache.go` `warmFrame`, wrap both `Rasterize` calls with `renderRestSVG`:
```go
	buf, err := Rasterize(renderRestSVG(data), w, h) // expensive – NO lock held
	if err != nil {
		if !fromDisk {
			return
		}
		def, ok := defaultBytes(name)
		if !ok {
			return
		}
		buf, err = Rasterize(renderRestSVG(def), w, h)
		if err != nil {
			return
		}
	}
```
- [ ] In `internal/face/cache.go` `Warm`, remove the speaking skip (the `speaking_*` frames no longer exist after Task 7, and `speaking` resolves to `neutral` art anyway). Replace:
```go
	for _, name := range CanonicalNames {
		if name == ExprSpeaking {
			continue
		}
		c.warmFrame(name, w, h)
	}
```
with:
```go
	for _, name := range CanonicalNames {
		c.warmFrame(name, w, h)
	}
```
- [ ] Run it (expect PASS): `CGO_ENABLED=1 go test ./internal/face/ -run 'TestCacheFrameRendersTemplatedNeutral|TestRenderRestSVG' -v`
- [ ] Lint: `golangci-lint run ./...`
- [ ] Commit: `git commit -am "face: rasterize cache faces at rest so templated faces work; warm speaking"`

---

## Task 3 — Convert the 8 core-set SVGs to `m`-driven templates

Each template introduces `{{$m := or .m 0.0}}` at the top and drives the mouth open by `$m ∈ [0,1]`. `m=0` reproduces today's exact resting mouth (so the fidelity/pixel tests and static display are unchanged at rest). Per the project oksvg arc-sweep gotcha, the open mouth uses a quadratic Bézier (`Q`) lower-lip drop — **no SVG arcs** for the animated geometry. Eyes/brows are untouched. The lower control point drops by up to ~22 user-units at full open, matching the existing `speaking_*` proportions (speaking_3 lower lip sat ~19 units below the closed line). Preview each with `face.Rasterize(renderRestSVG(bytes), ...)` and at `m=1` via `renderAnimTemplate(bytes, "m", 1)`.

Mouth math used below (computed in the template): the resting mouth is a `Q` curve `M x0 y0 Q cx cy0 x1 y0`. The animated version keeps the endpoints fixed and moves the control-point Y down by `drop = 22*$m`, opening the mouth as the voice signal rises. For the wide rounded-rect mouths (`smile`, `excited`) the resting art is a closed lip shape; those two keep the lip outline static and add a Bézier-bounded inner gap that grows with `$m` (so they "talk" without redrawing the whole lip). To keep this plan tractable and byte-faithful at rest, **all eight** use the simplest faithful parameterization: the visible mouth stroke/curve's open amount scales with `$m`, and at `$m=0` the path is byte-equivalent to today's resting path.

> **Risk noted:** `smile.svg` and `excited.svg` use a multi-path closed-lip mouth (teeth + tongue + arcs). Re-parameterizing the whole lip is high-risk for rest fidelity. The chosen approach keeps their existing resting mouth paths verbatim and overlays an `$m`-scaled dark inner-gap Bézier between the lips, present only when `$m>0` (zero-area at rest). This guarantees byte-faithful rest while still animating.

**Files:** Modify `internal/face/assets/{neutral,happy,smile,excited,content,concerned,sad,angry}.svg`

- [ ] Write the per-asset render test FIRST — create `internal/face/anim_templates_test.go`:
```go
package face

import "testing"

func TestCoreTemplatesRenderRestAndOpen(t *testing.T) {
	lib := NewLibrary(t.TempDir()) // embedded assets only
	for _, name := range []string{
		ExprNeutral, ExprHappy, ExprSmile, ExprExcited,
		ExprContent, ExprConcerned, ExprSad, ExprAngry,
	} {
		data, ok := lib.rawBytes(name)
		if !ok {
			t.Fatalf("%s: no embedded bytes", name)
		}
		// Rest (no data) must rasterize.
		rest, err := Rasterize(renderRestSVG(data), 80, 60)
		if err != nil {
			t.Fatalf("%s: rest rasterize: %v", name, err)
		}
		// Full open (m=1) must rasterize and differ from rest.
		openSVG, err := renderAnimTemplate(data, "m", 1)
		if err != nil {
			t.Fatalf("%s: render m=1: %v", name, err)
		}
		open, err := Rasterize(openSVG, 80, 60)
		if err != nil {
			t.Fatalf("%s: open rasterize: %v", name, err)
		}
		if equalFrame(rest, open) {
			t.Fatalf("%s: rest and open frames identical (mouth not animating)", name)
		}
	}
}
```
(`equalFrame` already exists in `anim_defaults_test.go`.)
- [ ] Run it (expect FAIL — assets are not yet templates, so rest==open): `CGO_ENABLED=1 go test ./internal/face/ -run TestCoreTemplatesRenderRestAndOpen -v`
- [ ] Convert `internal/face/assets/neutral.svg` (resting mouth `M 116 111 Q 140 125 160 111`; drop control-Y by `22*$m`):
```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">{{$m := or .m 0.0}}
  <rect x="0" y="0" width="280" height="210" fill="#4ECBA8"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#4ECBA8"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#90e5c8"/>
  <circle cx="80"  cy="78" r="6.5" fill="#1a1a1a"/>
  <circle cx="199" cy="78" r="6.5" fill="#1a1a1a"/>
  <path d="M 116 111 Q 140 {{add 125.0 (mul 22.0 $m)}} 160 111" stroke="#1a1a1a" stroke-width="4" fill="none" stroke-linecap="round"/>
</svg>
```
  > **Template-func note:** Go `text/template` has no built-in `add`/`mul`. Either (a) precompute the value inline with a single funcless expression — NOT possible for arithmetic — or (b) register `add`/`mul` in `renderAnimTemplate`. The engine's `renderAnimTemplate` currently does `template.New("anim").Parse(...)`; extend it (and `renderRestSVG`) to register a tiny FuncMap. See the **FuncMap sub-step** below; do it before converting any asset.

- [ ] **FuncMap sub-step (do before converting assets):** add a shared `animFuncs` and use it in both `renderAnimTemplate` and `renderRestSVG` so templates can do arithmetic. In `internal/face/anim_frames.go`:
```go
import (
	"bytes"
	"fmt"
	"text/template"
)

var animFuncs = template.FuncMap{
	"add": func(a, b float64) float64 { return a + b },
	"sub": func(a, b float64) float64 { return a - b },
	"mul": func(a, b float64) float64 { return a * b },
}
```
  Then in `renderAnimTemplate`:
```go
	tmpl, err := template.New("anim").Funcs(animFuncs).Parse(string(data))
```
  And in `renderRestSVG`:
```go
	tmpl, err := template.New("rest").Funcs(animFuncs).Parse(string(data))
```
  Add a guard test to `anim_frames_test.go`:
```go
func TestAnimFuncsArithmetic(t *testing.T) {
	in := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10">` +
		`{{$m := or .m 0.0}}<rect y="{{add 125.0 (mul 22.0 $m)}}"/></svg>`)
	rest := renderRestSVG(in)
	if !bytes.Contains(rest, []byte(`y="125"`)) {
		t.Fatalf("rest arithmetic wrong: %s", rest)
	}
	open, err := renderAnimTemplate(in, "m", 1)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !bytes.Contains(open, []byte(`y="147"`)) {
		t.Fatalf("open arithmetic wrong: %s", open)
	}
}
```
  > **Rest-value note:** `add 125.0 (mul 22.0 0.0)` prints `125` (Go formats a whole float64 without a decimal point), so the resting path is `M 116 111 Q 140 125 160 111` — byte-equivalent to today. Confirm with the test above.

- [ ] Convert `internal/face/assets/happy.svg` (resting `M 108 111 Q 140 134 172 111`, control-Y 134, stroke 5):
```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">{{$m := or .m 0.0}}
  <rect x="0" y="0" width="280" height="210" fill="#4ECBA8"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#4ECBA8"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#90e5c8"/>
  <circle cx="80" cy="78" r="6.5" fill="#1a1a1a"/>
  <circle cx="199" cy="78" r="6.5" fill="#1a1a1a"/>
  <path d="M 108 111 Q 140 {{add 134.0 (mul 18.0 $m)}} 172 111" stroke="#1a1a1a" stroke-width="5" fill="none" stroke-linecap="round"/>
</svg>
```
- [ ] Convert `internal/face/assets/content.svg` (resting mouth `M 116 111 Q 140 125 160 111`, eyes are arcs above — leave eyes verbatim):
```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">{{$m := or .m 0.0}}
  <rect x="0" y="0" width="280" height="210" fill="#4ECBA8"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#4ECBA8"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#90e5c8"/>
  <path d="M 61 73 Q 80 88 99 73" stroke="#1a1a1a" stroke-width="7" fill="none" stroke-linecap="round"/>
  <path d="M 180 73 Q 199 88 218 73" stroke="#1a1a1a" stroke-width="7" fill="none" stroke-linecap="round"/>
  <path d="M 116 111 Q 140 {{add 125.0 (mul 22.0 $m)}} 160 111" stroke="#1a1a1a" stroke-width="4" fill="none" stroke-linecap="round"/>
</svg>
```
- [ ] Convert `internal/face/assets/sad.svg` (resting mouth is a frown `M 116 113 Q 140 96 160 113`; "talking" means the lower lip drops toward neutral — increase control-Y by `26*$m` so at `m=1` it reads as an open mouth `~122`):
```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">{{$m := or .m 0.0}}
  <rect x="0" y="0" width="280" height="210" fill="#4ECBA8"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#4ECBA8"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#90e5c8"/>
  <circle cx="80" cy="78" r="6.5" fill="#1a1a1a"/>
  <circle cx="199" cy="78" r="6.5" fill="#1a1a1a"/>
  <path d="M 116 113 Q 140 {{add 96.0 (mul 26.0 $m)}} 160 113" stroke="#1a1a1a" stroke-width="4" fill="none" stroke-linecap="round"/>
</svg>
```
- [ ] Convert `internal/face/assets/concerned.svg` (resting mouth `M 116 111 Q 140 97 160 111`, plus angled brows — leave brows verbatim; drop control-Y by `24*$m`):
```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">{{$m := or .m 0.0}}
  <rect x="0" y="0" width="280" height="210" fill="#4ECBA8"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#4ECBA8"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#90e5c8"/>
  <circle cx="80"  cy="82" r="6.5" fill="#1a1a1a"/>
  <circle cx="199" cy="82" r="6.5" fill="#1a1a1a"/>
  <path d="M 65 54 L 96 70"   stroke="#1a1a1a" stroke-width="5" stroke-linecap="round"/>
  <path d="M 183 70 L 214 54" stroke="#1a1a1a" stroke-width="5" stroke-linecap="round"/>
  <path d="M 116 111 Q 140 {{add 97.0 (mul 24.0 $m)}} 160 111" stroke="#1a1a1a" stroke-width="4" fill="none" stroke-linecap="round"/>
</svg>
```
- [ ] Convert `internal/face/assets/angry.svg` (resting mouth `M 116 113 Q 140 99 160 113`, plus angled brows — leave brows verbatim; drop control-Y by `24*$m`):
```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">{{$m := or .m 0.0}}
  <rect x="0" y="0" width="280" height="210" fill="#4ECBA8"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#4ECBA8"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#90e5c8"/>
  <path d="M 60 60 L 96 74" stroke="#1a1a1a" stroke-width="5" stroke-linecap="round"/>
  <path d="M 183 74 L 219 60" stroke="#1a1a1a" stroke-width="5" stroke-linecap="round"/>
  <circle cx="80" cy="78" r="6.5" fill="#1a1a1a"/>
  <circle cx="199" cy="78" r="6.5" fill="#1a1a1a"/>
  <path d="M 116 113 Q 140 {{add 99.0 (mul 24.0 $m)}} 160 113" stroke="#1a1a1a" stroke-width="4" fill="none" stroke-linecap="round"/>
</svg>
```
- [ ] Convert `internal/face/assets/smile.svg`. Keep the closed-lip mouth paths VERBATIM (lip rect, teeth, tongue, lower lip). Add an `$m`-scaled dark inner-gap Bézier between the lips, drawn first so the lip paths overlay its edges, with zero vertical extent at `$m=0`. The lip interior spans roughly `x∈[106,174]`, top edge `y≈101`; open the gap downward by `28*$m` from a baseline at `y=118`:
```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">{{$m := or .m 0.0}}
  <rect x="0" y="0" width="280" height="210" fill="#4ECBA8"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#4ECBA8"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#90e5c8"/>
  <path d="M 61 85 Q 80 70 99 85"    stroke="#1a1a1a" stroke-width="7" fill="none" stroke-linecap="round"/>
  <path d="M 180 85 Q 199 70 218 85" stroke="#1a1a1a" stroke-width="7" fill="none" stroke-linecap="round"/>
  <path d="M 112 118 Q 140 {{add 118.0 (mul 28.0 $m)}} 168 118 Z" fill="#1a1a1a"/>
  <rect x="98" y="101" width="84" height="43" rx="20" ry="20" fill="#1a1a1a"/>
  <path d="M 99.67 113 A 20 20 0 0 1 118 101 L 162 101 A 20 20 0 0 1 180.33 113 Z" fill="#e4e4e4"/>
  <path d="M 99.67 113 L 180.33 113 A 20 20 0 0 1 182 121 L 182 124 A 20 20 0 0 1 162 144 L 118 144 A 20 20 0 0 1 98 124 L 98 121 A 20 20 0 0 1 99.67 113 Z" fill="#1a7848"/>
  <path d="M 116 144 A 24 8 0 0 1 164 144 Z" fill="#16ae81"/>
</svg>
```
  > **Note:** the existing static arc-based lip paths are PRESERVED unchanged (rest fidelity), and they are drawn AFTER the new gap path so they fully cover it at `$m=0` (gap is a degenerate zero-height Bézier). The pixel-fidelity sample for `smile` (`assets_test.go`) is at the eyes only, so rest fidelity holds. This is the spec-permitted use of arcs for STATIC geometry; the animated element (the gap) is a Bézier.

- [ ] Convert `internal/face/assets/excited.svg` (same closed-lip mouth as smile, with star eyes — keep eyes + lip paths verbatim, add the same `$m`-scaled gap):
```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">{{$m := or .m 0.0}}
  <rect x="0" y="0" width="280" height="210" fill="#4ECBA8"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#4ECBA8"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#90e5c8"/>
  <path d="M 80.0 65.0 L 83.1 73.8 L 92.4 74.0 L 84.9 79.6 L 87.6 88.5 L 80.0 83.2 L 72.4 88.5 L 75.1 79.6 L 67.6 74.0 L 76.9 73.8 Z" fill="#f4c531"/>
  <path d="M 199.0 65.0 L 202.1 73.8 L 211.4 74.0 L 203.9 79.6 L 206.6 88.5 L 199.0 83.2 L 191.4 88.5 L 194.1 79.6 L 186.6 74.0 L 195.9 73.8 Z" fill="#f4c531"/>
  <path d="M 112 118 Q 140 {{add 118.0 (mul 28.0 $m)}} 168 118 Z" fill="#1a1a1a"/>
  <rect x="98" y="101" width="84" height="43" rx="20" ry="20" fill="#1a1a1a"/>
  <path d="M 99.67 113 A 20 20 0 0 1 118 101 L 162 101 A 20 20 0 0 1 180.33 113 Z" fill="#e4e4e4"/>
  <path d="M 99.67 113 L 180.33 113 A 20 20 0 0 1 182 121 L 182 124 A 20 20 0 0 1 162 144 L 118 144 A 20 20 0 0 1 98 124 L 98 121 A 20 20 0 0 1 99.67 113 Z" fill="#1a7848"/>
  <path d="M 116 144 A 24 8 0 0 1 164 144 Z" fill="#16ae81"/>
</svg>
```
- [ ] Run it (expect PASS): `CGO_ENABLED=1 go test ./internal/face/ -run 'TestCoreTemplatesRenderRestAndOpen|TestAnimFuncsArithmetic|TestCacheFrameRendersTemplatedNeutral' -v`
- [ ] Lint: `golangci-lint run ./...`
- [ ] Commit: `git commit -am "face: template core-set mouths on param m (rest=today, opens with voice)"`

---

## Task 4 — Add core-set animation defs to `DefaultAnimations`

**Files:**
- Modify `internal/face/anim_defaults.go`
- Modify `internal/face/anim_defaults_test.go`

- [ ] Replace the two existing speaking tests in `internal/face/anim_defaults_test.go` with core-set equivalents (the old ones reference the retired `speaking` def):
```go
package face

import "testing"

func TestDefaultCoreAnimations(t *testing.T) {
	defs := DefaultAnimations()
	for _, name := range []string{
		ExprNeutral, ExprHappy, ExprSmile, ExprExcited,
		ExprContent, ExprConcerned, ExprSad, ExprAngry,
	} {
		d, ok := defs[name]
		if !ok {
			t.Fatalf("%s: default animation missing", name)
		}
		if d.Template == nil {
			t.Fatalf("%s: expected a template def", name)
		}
		if d.Template.Param != "m" || d.Template.Steps != 6 {
			t.Fatalf("%s: template=%+v", name, *d.Template)
		}
		if d.Driver.Kind != DriverAmplitude || d.Driver.Curve != curveSqrt {
			t.Fatalf("%s: driver=%+v", name, d.Driver)
		}
		if d.Driver.Idle != nil {
			t.Fatalf("%s: core defs must have NO idle (rest at silence)", name)
		}
	}
	if _, ok := defs[ExprSpeaking]; ok {
		t.Fatal("standalone speaking def should be retired")
	}
}

func TestDefaultCoreFramesRasterizeAndDiffer(t *testing.T) {
	lib := NewLibrary(t.TempDir())
	for _, name := range []string{ExprNeutral, ExprHappy, ExprSad} {
		frames, err := buildFrames(lib, DefaultAnimations()[name], 80, 60)
		if err != nil {
			t.Fatalf("%s: buildFrames: %v", name, err)
		}
		if len(frames) != 6 {
			t.Fatalf("%s: frames=%d want 6", name, len(frames))
		}
		if equalFrame(frames[0], frames[5]) {
			t.Fatalf("%s: rest and open frames identical", name)
		}
	}
}

func equalFrame(a, b []uint32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
```
  > Keep exactly one `equalFrame` definition in the package. If Task 3's `anim_templates_test.go` also defined it, define it only here and remove the duplicate.
- [ ] Run it (expect FAIL — defs not yet added): `CGO_ENABLED=1 go test ./internal/face/ -run 'TestDefaultCore' -v`
- [ ] Rewrite `internal/face/anim_defaults.go`:
```go
package face

// DefaultAnimations returns the built-in animation set baked into the binary.
// Overlay mods inherit these; self-contained mods do not (they declare their
// own). Each core-set emotion is a six-step template whose mouth is driven by
// the voice amplitude on param "m" (0 = rest, 1 = fully open). With NO Idle the
// amplitude driver rests at frame 0 during silence and opens the mouth as the
// signal rises, so the same face that shows the emotion also "talks". Built-in
// faces are therefore the reference implementation modders copy.
func DefaultAnimations() map[string]AnimationDef {
	tmpl := func(file string) AnimationDef {
		return AnimationDef{
			Template: &TemplateSource{File: file, Param: "m", From: 0, To: 1, Steps: 6},
			Driver:   Driver{Kind: DriverAmplitude, Curve: curveSqrt},
		}
	}
	return map[string]AnimationDef{
		ExprNeutral:   tmpl(ExprNeutral),
		ExprHappy:     tmpl(ExprHappy),
		ExprSmile:     tmpl(ExprSmile),
		ExprExcited:   tmpl(ExprExcited),
		ExprContent:   tmpl(ExprContent),
		ExprConcerned: tmpl(ExprConcerned),
		ExprSad:       tmpl(ExprSad),
		ExprAngry:     tmpl(ExprAngry),
	}
}
```
- [ ] Run it (expect PASS): `CGO_ENABLED=1 go test ./internal/face/ -run 'TestDefaultCore' -v`
- [ ] Lint: `golangci-lint run ./...`
- [ ] Commit: `git commit -am "face: add amplitude-driven default animations for core emotion set"`

---

## Task 5 — Wire speaking-state rendering to the templated neutral face

**Files:**
- Modify `cmd/bmo-pak/main.go` (lines 291, 317, 539, 612, 639)

- [ ] In `cmd/bmo-pak/main.go`, change the StateSpeaking no-emotion fallback (line ~612) from `ExpressionSpeaking` to `ExpressionNeutral`:
```go
		case assistant.StateSpeaking:
			errorSince = time.Time{}
			if snap.Emotion != "" {
				expr = string(snap.Emotion)
			} else {
				expr = string(assistant.ExpressionNeutral)
			}
```
- [ ] Change the clipPlaying override (line ~639) from `ExpressionSpeaking` to `ExpressionNeutral`:
```go
		if clipPlaying {
			expr = string(assistant.ExpressionNeutral)
		}
```
- [ ] Change both Prewarm calls (lines ~291 and ~317) from `face.ExprSpeaking` to `face.ExprNeutral`:
```go
		go animEngine.Prewarm(face.ExprNeutral, w, h)
```
- [ ] Change the startup readiness gate (line ~539) from `face.ExprSpeaking` to `face.ExprNeutral`:
```go
			(animEngine.Ready(face.ExprNeutral) || time.Since(startupFaceShownAt) > 10*time.Second) {
```
- [ ] Build (expect PASS): `CGO_ENABLED=1 go build ./...`
- [ ] Run the full face + assistant suites (expect PASS): `CGO_ENABLED=1 go test ./internal/face/ ./internal/assistant/ ./cmd/bmo-pak/`
- [ ] Lint: `golangci-lint run ./...`
- [ ] Commit: `git commit -am "bmo-pak: render speaking state as the amplitude-driven neutral face"`

---

## Task 6 — Drop the `laugh` emotion

**Files:**
- Modify `internal/face/expr.go` (const line 21, `CanonicalNames` line 51, `Canonical` case lines 83-84)
- Modify `internal/face/fidelity_test.go` (line 15) and `internal/face/testdata/approved_expressions.json`
- Modify `internal/face/assets_test.go` (line 91)
- Modify `internal/assistant/state.go` (line 64)
- Delete `internal/face/assets/laugh.svg`

- [ ] Update the fidelity test FIRST so it stops requiring `laugh` (and, anticipating Task 7's manifest edit, leave the five templated emotions to Task 7). In `internal/face/fidelity_test.go` remove `ExprLaugh` from `newExpressions`:
```go
var newExpressions = []string{
	ExprSad, ExprHappy, ExprContent, ExprAngry, ExprSurprised,
	ExprExcited, ExprLove, ExprShy, ExprCrying, ExprTeary, ExprGloomy,
	ExprDizzy, ExprUnamused, ExprAnnoyed, ExprSkeptical, ExprPlayful,
	ExprKiss, ExprGrimace, ExprShout, ExprDead, ExprGlitch, ExprDismayed,
	ExprAdoring, ExprSparkle,
}
```
  > Note: `ExprSad`, `ExprHappy`, `ExprContent`, `ExprAngry`, `ExprExcited` are intentionally still listed here — Task 7 removes them along with the manifest entries. This task only removes `laugh`.
- [ ] Remove the `laugh` entry from `internal/face/testdata/approved_expressions.json` (the manifest is a JSON object keyed by expression name; delete the `"laugh": "<sha256>"` member). Confirm the file remains valid JSON.
- [ ] Remove the `ExprLaugh` case from `internal/face/assets_test.go` (delete line 91 entirely).
- [ ] Run the face suite (expect FAIL only on the still-present `ExprLaugh` const references in `expr.go`/`state.go`, or PASS if those compile — proceed to remove the const next): `CGO_ENABLED=1 go test ./internal/face/`
- [ ] In `internal/face/expr.go` remove `ExprLaugh = "laugh"` from the const block, remove `ExprLaugh` from the `CanonicalNames` slice (line 51), and remove the `case ExprLaugh: return ExprLaugh` from `Canonical`.
- [ ] In `internal/assistant/state.go` remove `ExpressionLaugh Expression = "laugh"` (line 64).
- [ ] Delete the asset: `git rm internal/face/assets/laugh.svg`
- [ ] Update `internal/assistant/idle_test.go` (lines 28, 47, 61) — replace `ExpressionLaugh` with a pool member that survives Task 7's widening (use `ExpressionContent`). The exact replacement is finalized in Task 7; for now, swap `ExpressionLaugh → ExpressionContent` so the package compiles:
  - line 28 list: replace `ExpressionLaugh` with `ExpressionContent`
  - line 47: `counts[ExpressionBlink] <= counts[ExpressionContent]`
  - line 61: `counts[ExpressionContent] + counts[ExpressionSleeping]`
  > These assertions are revisited in Task 7 once pools are finalized; keep them consistent with the default pool there.
- [ ] Build (expect PASS): `CGO_ENABLED=1 go build ./...`
- [ ] Run (expect PASS): `CGO_ENABLED=1 go test ./internal/face/ ./internal/assistant/`
- [ ] Lint: `golangci-lint run ./...`
- [ ] Commit: `git commit -am "face,assistant: drop the laugh emotion from the vocabulary"`

---

## Task 7 — Widen idle pools to cycle the core emotion set

**Files:**
- Modify `internal/assistant/state.go` (verify/add `ExpressionExcited`, `ExpressionContent`, `ExpressionConcerned`, `ExpressionHappy`)
- Modify `internal/assistant/idle.go` (`poolFor`)
- Modify `internal/assistant/idle_test.go`

- [ ] Verify the needed Expression constants exist in `internal/assistant/state.go`. As read today: `ExpressionConcerned` (line 61), `ExpressionContent` (line 71), `ExpressionExcited` (line 74), `ExpressionHappy` (line 70) all already exist. **No new constants are required.** (If a future edit removed one, re-add it as `ExpressionX Expression = "x"`.)
- [ ] Write/adjust the failing idle test in `internal/assistant/idle_test.go` to assert the widened pools include the new core emotions and never include `laugh`:
```go
func TestIdlePoolsIncludeCoreEmotions(t *testing.T) {
	s := NewIdleScheduler(1)
	seen := map[Expression]bool{}
	for i := 0; i < 4000; i++ {
		// Sample across the time bands by advancing idleFor.
		step := s.Next(time.Duration(i%40) * time.Second)
		seen[step.Expression] = true
	}
	for _, want := range []Expression{
		ExpressionContent, ExpressionExcited, ExpressionConcerned, ExpressionHappy,
	} {
		if !seen[want] {
			t.Errorf("idle pools never produced %q", want)
		}
	}
	if seen["laugh"] {
		t.Error("laugh must no longer appear in idle pools")
	}
}
```
- [ ] Run it (expect FAIL — pools not yet widened): `CGO_ENABLED=1 go test ./internal/assistant/ -run TestIdlePoolsIncludeCoreEmotions -v`
- [ ] Rewrite `poolFor` in `internal/assistant/idle.go`:
```go
func (s *IdleScheduler) poolFor(idleFor time.Duration) ([]Expression, time.Duration) {
	switch {
	case idleFor < 2*time.Second:
		return []Expression{ExpressionBlink, ExpressionNeutral, ExpressionBlink, ExpressionBlink}, 1800 * time.Millisecond
	case idleFor < 8*time.Second:
		return []Expression{ExpressionBlink, ExpressionLookAround, ExpressionNeutral, ExpressionSmile, ExpressionHappy, ExpressionBlink, ExpressionWhistle}, 3000 * time.Millisecond
	case idleFor < 25*time.Second:
		return []Expression{ExpressionBlink, ExpressionLookAround, ExpressionSmile, ExpressionWhistle, ExpressionContent, ExpressionExcited, ExpressionNeutral}, 4500 * time.Millisecond
	default:
		return []Expression{ExpressionBlink, ExpressionLookAround, ExpressionSmile, ExpressionWhistle, ExpressionContent, ExpressionExcited, ExpressionConcerned, ExpressionSleeping, ExpressionBlink, ExpressionBlink}, 6000 * time.Millisecond
	}
}
```
- [ ] Reconcile the pre-existing `idle_test.go` assertions (lines 28/47/61, edited in Task 6) with the new default pool. The default pool now contains `ExpressionContent`, `ExpressionExcited`, `ExpressionConcerned`, `ExpressionSleeping`. Update the "large hold" assertions to reference members that are actually in the default pool (e.g. `ExpressionContent` and `ExpressionSleeping`), keeping the original intent (blink dominates short bands; long-hold faces dominate the default band). Re-read the test and make the member lists match `poolFor`.
- [ ] Run it (expect PASS): `CGO_ENABLED=1 go test ./internal/assistant/ -run 'TestIdle' -v`
- [ ] Lint: `golangci-lint run ./...`
- [ ] Commit: `git commit -am "assistant: widen idle pools to cycle core emotions; remove laugh"`

---

## Task 8 — Retire the standalone `speaking_*` assets

Done last so nothing references them. The `speaking` def is already removed (Task 4) and rendering maps speaking→neutral (Task 5).

**Files:**
- Delete `internal/face/assets/speaking_0.svg` … `speaking_5.svg` and `speaking.svg`
- Verify `cmd/gen-speaking-frames/main.go` (the generator) — leave or remove

- [ ] Confirm no Go code references the `speaking_*` basenames: `grep -rn "speaking_" --include="*.go" .` — expected hits only in `cmd/gen-speaking-frames/main.go` (the now-obsolete generator) and none in `internal/face` after Task 4.
- [ ] Delete the assets:
```
git rm internal/face/assets/speaking_0.svg internal/face/assets/speaking_1.svg internal/face/assets/speaking_2.svg internal/face/assets/speaking_3.svg internal/face/assets/speaking_4.svg internal/face/assets/speaking_5.svg internal/face/assets/speaking.svg
```
- [ ] Decide on `cmd/gen-speaking-frames`: it only generated the now-deleted frames. Remove it (`git rm -r cmd/gen-speaking-frames`) since its output is retired; this keeps the build honest. Confirm nothing imports it: `grep -rn "gen-speaking-frames" --include="*.go" .` (expected: none).
- [ ] Now handle the `speaking` resolution: `face.Canonical("speaking")` still returns `ExprSpeaking` and `lib.Bytes("speaking")` will call `defaultBytes("speaking")`. Since `speaking.svg` is deleted, `defaultBytes("speaking")` returns `(nil,false)`. Because nothing renders raw `speaking` anymore (Task 5 maps it to `neutral`), this is acceptable — BUT add a guard so a stray `speaking` request still draws a face. In `internal/face/expr.go` `Canonical`, change the speaking case to fold to neutral:
```go
	case ExprSpeaking:
		return ExprNeutral
```
  Keep the `ExprSpeaking` const (it is still used by `FunctionalNames` and `CanonicalNames`). Update the `CanonicalNames` list: leave `ExprSpeaking` in it is now harmless only if `defaultBytes` has a file; since the file is gone, REMOVE `ExprSpeaking` from `CanonicalNames` (line 49) so `Cache.Warm` does not try to rasterize a missing asset. `FunctionalNames` keeps `ExprSpeaking` (it is excluded from the emotion vocabulary regardless).
- [ ] Add/adjust an expr test in `internal/face/expr_test.go` to assert `Canonical("speaking") == ExprNeutral` (replace any prior assertion that speaking maps to itself).
- [ ] Build (expect PASS): `CGO_ENABLED=1 go build ./...`
- [ ] Run the full suite (expect PASS): `CGO_ENABLED=1 go test ./...`
- [ ] Lint: `golangci-lint run ./...`
- [ ] Commit: `git commit -am "face: retire standalone speaking frames; fold speaking to neutral"`

---

## Task 9 — Finalize fidelity manifest for templated emotions

The five core emotions that are also in `newExpressions` (`happy, content, angry, sad, excited`) are now Go templates, not frozen byte-art, so `TestNewExpressionFidelity` must stop guarding them.

**Files:**
- Modify `internal/face/fidelity_test.go`
- Modify `internal/face/testdata/approved_expressions.json`
- Modify `internal/face/assets_test.go`

- [ ] In `internal/face/fidelity_test.go`, remove `ExprSad, ExprHappy, ExprContent, ExprAngry, ExprExcited` from `newExpressions`, leaving only the still-static Figma faces:
```go
var newExpressions = []string{
	ExprSurprised, ExprLove, ExprShy, ExprCrying, ExprTeary, ExprGloomy,
	ExprDizzy, ExprUnamused, ExprAnnoyed, ExprSkeptical, ExprPlayful,
	ExprKiss, ExprGrimace, ExprShout, ExprDead, ExprGlitch, ExprDismayed,
	ExprAdoring, ExprSparkle,
}
```
- [ ] Remove the `happy`, `content`, `angry`, `sad`, `excited` members from `internal/face/testdata/approved_expressions.json`. Confirm valid JSON.
- [ ] In `internal/face/assets_test.go`, the pixel-fidelity sampler rasterizes `defaultBytes(name)` directly. For the templated core-set cases (`ExprNeutral`, `ExprConcerned`, `ExprSmile`, `ExprHappy`, `ExprContent`, `ExprAngry`, `ExprSad`, `ExprExcited`) the raw bytes now contain `{{`, so route them through `renderRestSVG` before `Rasterize`:
```go
			buf, err := Rasterize(renderRestSVG(data), w, h)
```
  (This single change covers every case in the table; non-template faces pass through `renderRestSVG` unchanged. The sampled points are eyes/brows/teeth which are static at rest, so the assertions hold.)
- [ ] Run the face suite (expect PASS): `CGO_ENABLED=1 go test ./internal/face/ -v`
- [ ] Lint: `golangci-lint run ./...`
- [ ] Commit: `git commit -am "face: stop byte-freezing templated core emotions; sample at rest"`

---

## Task 10 — Final full-suite + lint + regression guard

**Files:**
- Create/extend `internal/face/anim_engine_test.go` (regression guard)

- [ ] Add a regression-guard test asserting that, with an emotion set and a positive amplitude signal, the engine returns an animated frame distinct from the rest frame (this is the exact regression WS1 fixes):
```go
func TestSpeakingEmotionAnimates(t *testing.T) {
	lib := NewLibrary(t.TempDir())
	eng := NewEngine(lib, DefaultAnimations())
	w, h := 80, 60
	eng.Prewarm(ExprNeutral, w, h)
	// Build is async; spin until ready (bounded).
	for i := 0; i < 200 && !eng.Ready(ExprNeutral); i++ {
		time.Sleep(5 * time.Millisecond)
	}
	if !eng.Ready(ExprNeutral) {
		t.Fatal("neutral animation never became ready")
	}
	rest, ok := eng.AnimFrame(ExprNeutral, w, h, 0, 0, 0)   // silence
	if !ok {
		t.Fatal("rest AnimFrame not ok")
	}
	loud, ok := eng.AnimFrame(ExprNeutral, w, h, 0, 0, 1.0) // full voice
	if !ok {
		t.Fatal("loud AnimFrame not ok")
	}
	if equalFrame(rest, loud) {
		t.Fatal("mouth did not move between silence and full voice (regression)")
	}
}
```
  (Add `"time"` import. Confirm `Engine.AnimFrame`/`Ready`/`Prewarm` signatures match `anim_engine.go`: `AnimFrame(expr string, w, h int, clock, epoch float64, signal float32) ([]uint32, bool)`.)
- [ ] Run it (expect PASS): `CGO_ENABLED=1 go test ./internal/face/ -run TestSpeakingEmotionAnimates -v`
- [ ] Full suite (expect PASS): `CGO_ENABLED=1 go test ./...`
- [ ] Race check on the engine (async build): `CGO_ENABLED=1 go test -race ./internal/face/`
- [ ] Lint: `golangci-lint run ./...`
- [ ] Build: `CGO_ENABLED=1 go build ./...`
- [ ] Commit: `git commit -am "face: regression guard — neutral mouth animates with voice signal"`

---

## Self-Review

**Spec WS1 coverage:**
1. **Animate built-ins through the engine (dogfood).** Tasks 3 + 4 template the 8 core SVGs on param `m` and register amplitude `Template` defs in `DefaultAnimations()`, the exact mechanism mods use. Built-ins are now the modder reference. ✔
2. **Cache rest-rendering.** Task 1 (`renderRestSVG`) + Task 2 (cache routes through it) + Task 9 (tests route through it) ensure templated faces rasterize at rest for static display. ✔
3. **Core-set defs (no Idle = rest at silence, animate with voice).** Task 4 defs use `DriverAmplitude` + `curveSqrt` + NO `Idle`; `Driver.Step` returns frame 0 at `signal<=0`. ✔
4. **Speaking = templated neutral.** Task 4 retires the standalone `speaking` def; Task 5 maps StateSpeaking (and clipPlaying) to `neutral`, repoints Prewarm/Ready to `ExprNeutral`; Task 8 folds `Canonical("speaking")→neutral` and deletes `speaking_*`. Decision recorded: `ExprSpeaking`/`ExpressionSpeaking` are kept as state labels (still referenced by `FunctionalNames` and the state machine), per the spec's "remove once no longer referenced." ✔
5. **Drop laugh.** Task 6 removes `laugh.svg`, `ExprLaugh` (const/`CanonicalNames`/`Canonical`), `ExpressionLaugh`, manifest entry, and test references. ✔
6. **Widen idle pools.** Task 7 adds `content, excited, concerned, happy` to the pools; verified no new Expression constants are needed. ✔
7. **Prewarm neutral.** Task 5 changes both Prewarm calls and the Ready gate to `ExprNeutral`. ✔

**Testing bullets (spec WS1 testing section):**
- *Driver:* Task 10 regression guard proves silence→rest, full-voice→open differ. ✔
- *Template render:* Task 3 asserts every core template rasterizes at rest and at `m=1` and that rest≠open. ✔
- *Cache static:* Task 2 asserts `Cache.Frame(neutral)` returns a non-nil buffer (template rasterized at rest). ✔
- *Idle:* Task 7 asserts pools include the new core emotions and never `laugh`. ✔
- *Regression guard:* Task 10 `TestSpeakingEmotionAnimates`. ✔

**Ordering keeps the build green:** rest-helper → cache wiring → asset templating (tests added before each conversion) → defs → main.go wiring → drop laugh → widen pools → retire speaking_* → finalize fidelity manifest → full suite. Each task ends on a passing build + lint + commit.

**Risks / caveats encountered while drafting:**
- `smile.svg` and `excited.svg` have a multi-path **arc-based** closed-lip mouth (teeth/tongue). Re-parameterizing the whole lip risked breaking rest byte/pixel fidelity, so the plan keeps those lip paths verbatim and overlays a Bézier inner-gap that is zero-area at rest and grows with `m` — animated geometry stays Béziers (oksvg arc-sweep gotcha respected) while static arcs are preserved.
- `text/template` has no arithmetic; the plan adds a small `add`/`mul`/`sub` FuncMap to both `renderAnimTemplate` and `renderRestSVG` (Task 3 FuncMap sub-step). Whole-number float64 values format without a decimal point, so rest paths stay byte-equivalent to today (verified by `TestAnimFuncsArithmetic`).
- `TestNewExpressionFidelity` byte-freezes art; the five templated core emotions and `laugh` must be removed from `newExpressions` AND the `approved_expressions.json` manifest (Tasks 6 + 9), or the suite fails.
- `assets_test.go` rasterizes raw bytes and pixel-samples; it must route core-set bytes through `renderRestSVG` (Task 9). Sampled points are eyes/brows/teeth (static at rest), so assertions hold.
- `ExprSpeaking` is intentionally NOT fully removed (still a state label / functional name); only `CanonicalNames` drops it (Task 8) to avoid `Cache.Warm` rasterizing the deleted `speaking.svg`.
