# Whistle & Look-Around Idle Animations Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the frozen-`neutral` fold for the `look_around` and `whistle` idle expressions with two gently animated, time-driven faces, kept out of the LLM emotion vocabulary.

**Architecture:** Add two templated SVG assets and register them in `face.DefaultAnimations()` with a `DriverTime` driver (no audio). Wire the expression names into `Canonical`, `CanonicalNames`, and `FunctionalNames` so the static cache warms a sensible resting fallback while the model is never told about them. The idle scheduler already references the names, so no assistant changes are needed.

**Tech Stack:** Go, `text/template`-driven SVG (`internal/face`), `oksvg` rasterizer. Tests run under `CGO_ENABLED=1 go test ./...`; lint via `golangci-lint run ./...`.

---

## File Structure

- `internal/face/expr.go` — add `ExprWhistle`/`ExprLookAround` constants, `Canonical` cases, `CanonicalNames` entries.
- `internal/face/emotion.go` — add both to `FunctionalNames` (excluded from LLM vocab).
- `internal/face/assets/look_around.svg` — **new** templated asset (param `x`: eye pan).
- `internal/face/assets/whistle.svg` — **new** templated asset (param `t`: rising note).
- `internal/face/anim_defaults.go` — register both `DriverTime` animations.
- `internal/face/expr_test.go` — canonical mappings.
- `internal/face/emotion_test.go` — vocab exclusion + canonical membership.
- `internal/face/anim_defaults_test.go` — animation def shape + frames rasterize & differ.
- `internal/face/assets_test.go` — rest-frame geometry for both new faces.
- Memory: `reference_whistle_lookaround_fold_to_neutral.md` rewrite + `MEMORY.md` line.

Notes for the implementer:
- The renderer (`internal/renderer/bmo.go:373`) passes the **raw** lowercased expression name to `Engine.AnimFrame`, so the animation key must equal the constant string (`"look_around"`/`"whistle"`).
- oksvg renders degenerate `<path>` arc (`A`) sweeps opposite to ImageMagick (`reference_oksvg_arc_sweep`). Use `<ellipse>`/`<rect>`/quadratic Béziers only in the new assets — **no `A` commands**.
- Template float output uses `printf "%.1f"`/`"%.2f"` to avoid float-noise in SVG numbers; `add`/`sub`/`mul` are provided by `animFuncs`.

---

### Task 1: Expression constants + Canonical resolution

**Files:**
- Modify: `internal/face/expr.go`
- Test: `internal/face/expr_test.go`

- [ ] **Step 1: Update the failing canonical test**

In `internal/face/expr_test.go`, change the existing `look_around` line and add whistle + alias. Find:

```go
		{"look_around", ExprNeutral},
```

Replace with:

```go
		{"look_around", ExprLookAround},
		{"lookaround", ExprLookAround},
		{"whistle", ExprWhistle},
```

- [ ] **Step 2: Run test to verify it fails (compile error)**

Run: `CGO_ENABLED=1 go test ./internal/face/ -run TestCanonical -v`
Expected: FAIL — `undefined: ExprLookAround` / `undefined: ExprWhistle`.

- [ ] **Step 3: Add the constants**

In `internal/face/expr.go`, in the `const (...)` block, after `ExprSparkle = "sparkle"` (the last entry before the closing `)`), add:

```go
	// Idle-only animated faces. Never requested by the model (see
	// FunctionalNames); driven by the idle scheduler in internal/assistant.
	ExprLookAround = "look_around"
	ExprWhistle    = "whistle"
```

- [ ] **Step 4: Add the Canonical cases**

In `internal/face/expr.go`, in `Canonical`, immediately before the `default:` case, add:

```go
	case ExprLookAround, "lookaround":
		return ExprLookAround
	case ExprWhistle:
		return ExprWhistle
```

- [ ] **Step 5: Run test to verify it passes**

Run: `CGO_ENABLED=1 go test ./internal/face/ -run TestCanonical -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/face/expr.go internal/face/expr_test.go
git commit -m "feat(face): canonicalize look_around and whistle expressions"
```

---

### Task 2: Look-around SVG asset + animation

**Files:**
- Create: `internal/face/assets/look_around.svg`
- Modify: `internal/face/anim_defaults.go`
- Test: `internal/face/anim_defaults_test.go`

- [ ] **Step 1: Write the failing animation test**

In `internal/face/anim_defaults_test.go`, append:

```go
func TestDefaultLookAroundAnimation(t *testing.T) {
	def, ok := DefaultAnimations()[ExprLookAround]
	if !ok {
		t.Fatal("look_around default missing")
	}
	if def.Template == nil || def.Template.Param != "x" || def.Template.Steps != 5 {
		t.Fatalf("template=%+v", def.Template)
	}
	if def.Driver.Kind != DriverTime || def.Driver.Mode != modePingpong || def.Driver.FPS != 3 {
		t.Fatalf("driver=%+v", def.Driver)
	}
	lib := NewLibrary(t.TempDir())
	frames, err := buildFrames(lib, def, 80, 60)
	if err != nil {
		t.Fatalf("buildFrames: %v", err)
	}
	if len(frames) != 5 {
		t.Fatalf("frames=%d want 5", len(frames))
	}
	if equalFrame(frames[0], frames[4]) {
		t.Fatal("eyes-left and eyes-right frames are identical")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `CGO_ENABLED=1 go test ./internal/face/ -run TestDefaultLookAroundAnimation -v`
Expected: FAIL — `look_around default missing` (and the asset is absent).

- [ ] **Step 3: Create the asset**

Create `internal/face/assets/look_around.svg`:

```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">{{$x := or .x 0.0}}{{$lx := add 80.0 (mul $x 14.0)}}{{$rx := add 199.0 (mul $x 14.0)}}
  <rect x="0" y="0" width="280" height="210" fill="#4ECBA8"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#4ECBA8"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#90e5c8"/>
  <circle cx="{{printf "%.1f" $lx}}" cy="78" r="6.5" fill="#1a1a1a"/>
  <circle cx="{{printf "%.1f" $rx}}" cy="78" r="6.5" fill="#1a1a1a"/>
  <path d="M 116 111 Q 140 125 160 111" stroke="#1a1a1a" stroke-width="4" fill="none" stroke-linecap="round"/>
</svg>
```

At rest (`x=0`) the eyes sit at cx 80.0 / 199.0 — visually identical to neutral, so the static fallback reads clean.

- [ ] **Step 4: Register the animation**

In `internal/face/anim_defaults.go`, inside the `return map[string]AnimationDef{ ... }` literal, after the `ExprSpeaking: { ... },` block and before the closing `}`, add:

```go
		// Idle-only, no audio: a gentle horizontal eye scan. Pingpong over
		// x ∈ {-1,-0.5,0,0.5,1} at 3 fps gives a ~2.7s left↔right sweep.
		ExprLookAround: {
			Template: &TemplateSource{File: ExprLookAround, Param: "x", From: -1, To: 1, Steps: 5},
			Driver:   Driver{Kind: DriverTime, FPS: 3, Mode: modePingpong},
		},
```

- [ ] **Step 5: Run test to verify it passes**

Run: `CGO_ENABLED=1 go test ./internal/face/ -run TestDefaultLookAroundAnimation -v`
Expected: PASS.

- [ ] **Step 6: Visually verify the asset (no oksvg arc artifacts)**

Run this throwaway preview and open the PNGs to confirm the eyes pan and nothing renders inverted:

```bash
cat > /tmp/prev_look.go <<'EOF'
package main

import (
	"image"
	"image/color"
	"image/png"
	"os"

	face "github.com/<module>/internal/face" // replace with the real module path from go.mod
)

func main() {
	lib := face.NewLibrary(os.TempDir())
	for i, x := range []float64{-1, 0, 1} {
		_ = i; _ = x; _ = lib
	}
	_ = color.RGBA{}; _ = image.Rect; _ = png.Encode
}
EOF
echo "If a quick Go preview is awkward, instead eyeball via the existing gallery: go run ./cmd/render-faces -h"
```

Practical alternative (preferred): rely on the rasterize-and-differ assertion in Step 1 plus a one-off `go run ./cmd/render-faces` (see Task 5) to inspect `look_around`. Do **not** leave `/tmp/prev_look.go` behind.

- [ ] **Step 7: Commit**

```bash
git add internal/face/assets/look_around.svg internal/face/anim_defaults.go internal/face/anim_defaults_test.go
git commit -m "feat(face): add look_around idle eye-scan animation"
```

---

### Task 3: Whistle SVG asset + animation

**Files:**
- Create: `internal/face/assets/whistle.svg`
- Modify: `internal/face/anim_defaults.go`
- Test: `internal/face/anim_defaults_test.go`

- [ ] **Step 1: Write the failing animation test**

In `internal/face/anim_defaults_test.go`, append:

```go
func TestDefaultWhistleAnimation(t *testing.T) {
	def, ok := DefaultAnimations()[ExprWhistle]
	if !ok {
		t.Fatal("whistle default missing")
	}
	if def.Template == nil || def.Template.Param != "t" || def.Template.Steps != 6 {
		t.Fatalf("template=%+v", def.Template)
	}
	if def.Driver.Kind != DriverTime || def.Driver.Mode != modeLoop || def.Driver.FPS != 4 {
		t.Fatalf("driver=%+v", def.Driver)
	}
	lib := NewLibrary(t.TempDir())
	frames, err := buildFrames(lib, def, 80, 60)
	if err != nil {
		t.Fatalf("buildFrames: %v", err)
	}
	if len(frames) != 6 {
		t.Fatalf("frames=%d want 6", len(frames))
	}
	if equalFrame(frames[0], frames[5]) {
		t.Fatal("note-low and note-high whistle frames are identical")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `CGO_ENABLED=1 go test ./internal/face/ -run TestDefaultWhistleAnimation -v`
Expected: FAIL — `whistle default missing`.

- [ ] **Step 3: Create the asset**

Create `internal/face/assets/whistle.svg`. The note head/stem/flag are computed from `$hx`/`$hy`; opacity fades as `t`→1. Only `<ellipse>`/`<rect>`/quadratic-Bézier `<path>` are used (no arcs):

```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">{{$t := or .t 0.0}}{{$hx := add 168.0 (mul $t 18.0)}}{{$hy := sub 80.0 (mul $t 40.0)}}{{$op := sub 1.0 $t}}
  <rect x="0" y="0" width="280" height="210" fill="#4ECBA8"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#4ECBA8"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#90e5c8"/>
  <circle cx="80" cy="78" r="6.5" fill="#1a1a1a"/>
  <circle cx="199" cy="78" r="6.5" fill="#1a1a1a"/>
  <ellipse cx="140" cy="116" rx="9" ry="9" fill="#1a1a1a"/>
  <ellipse cx="140" cy="116" rx="4.5" ry="4.5" fill="#1a7848"/>
  <ellipse cx="{{printf "%.1f" $hx}}" cy="{{printf "%.1f" $hy}}" rx="6.5" ry="5" fill="#1a1a1a" fill-opacity="{{printf "%.2f" $op}}"/>
  <rect x="{{printf "%.1f" (add $hx 4.5)}}" y="{{printf "%.1f" (sub $hy 25.0)}}" width="3" height="25" fill="#1a1a1a" fill-opacity="{{printf "%.2f" $op}}"/>
  <path d="M {{printf "%.1f" (add $hx 7.5)}} {{printf "%.1f" (sub $hy 25.0)}} Q {{printf "%.1f" (add $hx 17.0)}} {{printf "%.1f" (sub $hy 21.0)}} {{printf "%.1f" (add $hx 9.0)}} {{printf "%.1f" (sub $hy 13.0)}} Z" fill="#1a1a1a" fill-opacity="{{printf "%.2f" $op}}"/>
</svg>
```

At rest (`t=0`): pursed dark "o" mouth with a green interior, plus a fully-opaque note beside it — a distinct whistling still face.

- [ ] **Step 4: Register the animation**

In `internal/face/anim_defaults.go`, after the `ExprLookAround: { ... },` block added in Task 2, add:

```go
		// Idle-only, no audio: pursed mouth with a music note that floats up and
		// fades. Loop over t ∈ {0..1} at 4 fps → note rises every ~1.5s.
		ExprWhistle: {
			Template: &TemplateSource{File: ExprWhistle, Param: "t", From: 0, To: 1, Steps: 6},
			Driver:   Driver{Kind: DriverTime, FPS: 4, Mode: modeLoop},
		},
```

- [ ] **Step 5: Run test to verify it passes**

Run: `CGO_ENABLED=1 go test ./internal/face/ -run TestDefaultWhistleAnimation -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/face/assets/whistle.svg internal/face/anim_defaults.go internal/face/anim_defaults_test.go
git commit -m "feat(face): add whistle idle animation (pursed mouth + rising note)"
```

---

### Task 4: Vocabulary wiring (canonical membership + functional exclusion)

**Files:**
- Modify: `internal/face/expr.go` (`CanonicalNames`)
- Modify: `internal/face/emotion.go` (`FunctionalNames`)
- Modify: `internal/face/assets_test.go` (rest geometry)
- Test: `internal/face/emotion_test.go`

- [ ] **Step 1: Write the failing vocab test**

In `internal/face/emotion_test.go`, append:

```go
func TestWhistleLookAroundAreFunctionalIdleFaces(t *testing.T) {
	canon := map[string]bool{}
	for _, n := range CanonicalNames {
		canon[n] = true
	}
	for _, n := range []string{ExprLookAround, ExprWhistle} {
		if !canon[n] {
			t.Errorf("%q must be in CanonicalNames (warms static fallback)", n)
		}
		if !isFunctional(n) {
			t.Errorf("%q must be functional (excluded from the LLM vocab)", n)
		}
	}
	for _, n := range EmotionNames() {
		if n == ExprLookAround || n == ExprWhistle {
			t.Errorf("EmotionNames must not advertise idle face %q to the model", n)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `CGO_ENABLED=1 go test ./internal/face/ -run TestWhistleLookAroundAreFunctionalIdleFaces -v`
Expected: FAIL — not in `CanonicalNames`, not functional.

- [ ] **Step 3: Add to CanonicalNames**

In `internal/face/expr.go`, change the tail of the `CanonicalNames` slice. Find:

```go
	ExprAdoring, ExprSparkle,
}
```

Replace with:

```go
	ExprAdoring, ExprSparkle,
	// Idle-only animated faces (functional; excluded from the LLM vocab).
	ExprLookAround, ExprWhistle,
}
```

- [ ] **Step 4: Add to FunctionalNames**

In `internal/face/emotion.go`, find:

```go
var FunctionalNames = []string{ExprBlink, ExprListening, ExprThinking, ExprSpeaking, ExprSleeping}
```

Replace with:

```go
var FunctionalNames = []string{ExprBlink, ExprListening, ExprThinking, ExprSpeaking, ExprSleeping, ExprLookAround, ExprWhistle}
```

- [ ] **Step 5: Add rest-frame geometry assertions**

In `internal/face/assets_test.go`, in `TestNewFacesGeometry`, add two entries to the `cases` map (alongside the others):

```go
		ExprLookAround: {{80, 78, d, "left eye centered at rest"}, {199, 78, d, "right eye centered at rest"}},
		ExprWhistle:    {{80, 78, d, "left eye dot"}, {199, 78, d, "right eye dot"}, {140, 116, mi, "pursed mouth interior"}},
```

(`d` = dark features, `mi` = mouth interior `#1a7848`, already declared at the top of the test.)

- [ ] **Step 6: Run the affected tests to verify they pass**

Run: `CGO_ENABLED=1 go test ./internal/face/ -run 'TestWhistleLookAroundAreFunctionalIdleFaces|TestNewFacesGeometry|TestEmotionNamesExcludeFunctional' -v`
Expected: PASS (all).

- [ ] **Step 7: Commit**

```bash
git add internal/face/expr.go internal/face/emotion.go internal/face/assets_test.go internal/face/emotion_test.go
git commit -m "feat(face): make look_around/whistle canonical idle faces, out of LLM vocab"
```

---

### Task 5: Full verification + memory/doc bookkeeping

**Files:**
- Modify: memory `reference_whistle_lookaround_fold_to_neutral.md` + `MEMORY.md`
- (Optional) regenerate face gallery via `cmd/render-faces`

- [ ] **Step 1: Run the full face package tests**

Run: `CGO_ENABLED=1 go test ./internal/face/ -v 2>&1 | grep -vE '^=== RUN|Cannot process svg element'`
Expected: all `--- PASS` / `ok`.

- [ ] **Step 2: Run the whole suite and lint (verification gate)**

Run: `CGO_ENABLED=1 go test ./... 2>&1 | grep -vE '\[no test files\]|^ok|Cannot process svg element'; echo "(empty above = clean)"`
Then: `golangci-lint run ./...`
Expected: no failures, no new lint findings.

- [ ] **Step 3: (Optional) regenerate the face gallery**

If the repo's gallery should include the two new faces:

Run: `CGO_ENABLED=0 go run ./cmd/render-faces` (inspect output), then update `docs/images/faces` / gallery doc if the tool writes there. Skip if the gallery is curated/manual — this is not required for correctness.

- [ ] **Step 4: Update the obsolete memory**

The memory `reference_whistle_lookaround_fold_to_neutral.md` is now wrong. Rewrite its body to state that `look_around` and `whistle` are time-driven idle animations registered in `face.DefaultAnimations()` (`DriverTime`, pingpong/loop), are canonical **functional** faces (excluded from the LLM emotion vocab via `FunctionalNames`), and have templated assets `internal/face/assets/{look_around,whistle}.svg`. Update its `description:` accordingly and adjust the matching one-line entry in `MEMORY.md`.

- [ ] **Step 5: Commit any doc/gallery changes**

```bash
git add -A
git commit -m "docs(face): note look_around/whistle are animated idle faces"
```

(Memory files live outside the repo; save those with the Write tool, not git.)

---

## Self-Review

**Spec coverage:**
- Assets (look_around, whistle templated SVGs) → Tasks 2, 3. ✅
- Engine registration (`DriverTime`) → Tasks 2, 3. ✅
- Constants + `Canonical` cases → Task 1. ✅
- `CanonicalNames` + `FunctionalNames` wiring → Task 4. ✅
- Tests: canonical (T1), animation defs + rasterize/differ (T2/T3), vocab exclusion (T4), geometry (T4). ✅
- Verification gate `go test ./...` + `golangci-lint` → Task 5. ✅
- Memory bookkeeping → Task 5. ✅
- Non-goals (no bob, no LLM use, no engine changes) respected. ✅

**Placeholder scan:** The `/tmp/prev_look.go` snippet in Task 2 Step 6 is deliberately a throwaway and the step recommends the gallery alternative; no TBDs in shipped code/tests. Module path in that snippet must be filled from `go.mod` — but it is an optional preview, not shipped.

**Type consistency:** `ExprLookAround`/`ExprWhistle` (string consts) used identically across `expr.go`, `anim_defaults.go`, `emotion.go`, and tests; `TemplateSource{File,Param,From,To,Steps}` and `Driver{Kind,FPS,Mode}` match `anim_def.go`; `modePingpong`/`modeLoop`/`DriverTime` are the existing identifiers; `equalFrame`/`NewLibrary`/`buildFrames` exist in the package; `d`/`mi` color vars exist in `TestNewFacesGeometry`.
