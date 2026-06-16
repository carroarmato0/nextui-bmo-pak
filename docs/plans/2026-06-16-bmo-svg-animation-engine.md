# Declarative SVG Animation Engine Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace BMO's one hardcoded animated face (`speaking`) with a general, declarative SVG animation engine driven by time or lip-sync amplitude, and re-express `speaking` through it.

**Architecture:** A new animation layer in `internal/face`: a tolerant manifest parser (`AnimationDef`), two frame providers (explicit SVG-file list / parametric Go-template), two drivers (time / amplitude), and an `Engine` with an async, single-flight, scoped frame cache (only the active expression's frames resident). The renderer tries the engine first and falls back to the existing static-face path. The built-in `speaking` animation ships as six generated SVG frames + an embedded default definition; the bespoke `speak.go`/`Cache.Speak` machinery is deleted.

**Tech Stack:** Go; `text/template` for parametric SVGs; `oksvg`/`rasterx` (existing `face.Rasterize`) for rasterization; `encoding/json` for the manifest. Tests run `CGO_ENABLED=0 go test ./internal/face/...`; the renderer builds under CGO. Run `golangci-lint run ./...` before each commit.

---

## File Structure

- **Create `internal/face/anim_def.go`** — animation spec types (`AnimationDef`, `TemplateSource`, `Driver`, `Idle`) and the tolerant JSON parser (`ParseAnimations`, `parseAnimation`, `parseDriver`).
- **Create `internal/face/anim_driver.go`** — `Driver.Step` and the pure `timeStep` helper.
- **Create `internal/face/anim_frames.go`** — frame providers: `buildFrames`, `frameSVG`, `renderAnimTemplate`; plus `Library.rawBytes` literal-name loader (added to `library.go`).
- **Create `internal/face/anim_engine.go`** — `Engine`, `NewEngine`, `AnimFrame`, `Prewarm`, `Has`.
- **Create `internal/face/anim_defaults.go`** — `DefaultAnimations()` (the embedded `speaking` definition).
- **Create `cmd/gen-speaking-frames/main.go`** — offline generator that writes `speaking_0.svg`…`speaking_5.svg` and the static `speaking.svg`. Self-contained (its own copy of the mouth geometry).
- **Modify `internal/face/library.go`** — add `rawBytes`.
- **Modify `internal/face/cache.go`** + **delete `internal/face/speak.go`** — remove `Speak`, `warmSpeak`, `SpeakReady`, `speakSet`, `Strip`, `renderSpeakLevels`, `buildSpeakLocked`, `buildSpeakFromDefault`, `speakBand`, `speakParams`, `renderSpeakSVG`, `IsSpeakTemplate`.
- **Modify `internal/renderer/bmo.go`** — `SetAnimations`, expr-start epoch tracking, `blitFace` via engine, remove the speaking special-case and `blitStrip`.
- **Modify `internal/mod/manifest.go`** — add `Animations map[string]json.RawMessage`.
- **Modify `cmd/bmo-pak/main.go`** — build the effective animation set and install the engine at startup and in `reloadMod`.
- **Replace assets** `internal/face/assets/speaking.svg` and add `speaking_0.svg`…`speaking_5.svg`.

Each task is independently committable. Tasks 1–4 are pure `face` logic with no dependency on the renderer or mod wiring.

---

### Task 1: Animation spec types + tolerant parser

**Files:**
- Create: `internal/face/anim_def.go`
- Test: `internal/face/anim_def_test.go`

- [ ] **Step 1: Write the failing test**

```go
package face

import (
	"encoding/json"
	"testing"
)

func raw(s string) json.RawMessage { return json.RawMessage(s) }

func TestParseAnimationFramesForm(t *testing.T) {
	def, err := parseAnimation(raw(`{"frames":["a","b","c"],"driver":"amplitude"}`))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if def.Template != nil {
		t.Fatalf("expected list form, got template")
	}
	if got := def.Steps(); got != 3 {
		t.Fatalf("Steps()=%d want 3", got)
	}
	if def.Driver.Kind != DriverAmplitude || def.Driver.Curve != "linear" {
		t.Fatalf("driver=%+v want amplitude/linear", def.Driver)
	}
}

func TestParseAnimationTemplateForm(t *testing.T) {
	def, err := parseAnimation(raw(`{"template":"dots.svg","param":"V","from":0,"to":3,"steps":4,"driver":{"type":"time","fps":6,"mode":"loop"}}`))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if def.Template == nil || def.Template.Param != "V" || def.Template.Steps != 4 {
		t.Fatalf("template=%+v", def.Template)
	}
	if def.Driver.Kind != DriverTime || def.Driver.FPS != 6 || def.Driver.Mode != "loop" {
		t.Fatalf("driver=%+v", def.Driver)
	}
}

func TestParseAnimationAmplitudeObjectWithIdle(t *testing.T) {
	def, err := parseAnimation(raw(`{"frames":["a","b"],"driver":{"type":"amplitude","curve":"sqrt","idle":{"fps":13,"mode":"pingpong"}}}`))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if def.Driver.Curve != "sqrt" || def.Driver.Idle == nil || def.Driver.Idle.FPS != 13 || def.Driver.Idle.Mode != "pingpong" {
		t.Fatalf("driver=%+v idle=%+v", def.Driver, def.Driver.Idle)
	}
}

func TestParseAnimationRejectsBothSources(t *testing.T) {
	if _, err := parseAnimation(raw(`{"frames":["a"],"template":"x.svg","param":"V","steps":2,"driver":"amplitude"}`)); err == nil {
		t.Fatal("expected error for both frames and template")
	}
}

func TestParseAnimationRejectsNoDriver(t *testing.T) {
	if _, err := parseAnimation(raw(`{"frames":["a","b"]}`)); err == nil {
		t.Fatal("expected error for missing driver")
	}
}

func TestParseAnimationsSkipsMalformed(t *testing.T) {
	in := map[string]json.RawMessage{
		"good": raw(`{"frames":["a","b"],"driver":"amplitude"}`),
		"bad":  raw(`{"driver":"amplitude"}`), // no frame source
	}
	defs, errs := ParseAnimations(in)
	if _, ok := defs["good"]; !ok {
		t.Fatal("good animation missing")
	}
	if _, ok := defs["bad"]; ok {
		t.Fatal("bad animation should be skipped")
	}
	if len(errs) != 1 {
		t.Fatalf("len(errs)=%d want 1", len(errs))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `CGO_ENABLED=0 go test ./internal/face/ -run TestParseAnimation -v`
Expected: FAIL — `undefined: parseAnimation` etc.

- [ ] **Step 3: Write the implementation**

```go
package face

import (
	"encoding/json"
	"fmt"
)

// DriverKind enumerates the supported animation drivers.
type DriverKind string

const (
	DriverAmplitude DriverKind = "amplitude"
	DriverTime      DriverKind = "time"
)

// AnimationDef is a parsed, validated animation. Exactly one frame source is
// set: Frames (explicit SVG basenames) or Template (parametric).
type AnimationDef struct {
	Frames   []string
	Template *TemplateSource
	Driver   Driver
}

// TemplateSource renders one Go-template SVG at Steps samples of Param across
// [From, To].
type TemplateSource struct {
	File  string
	Param string
	From  float64
	To    float64
	Steps int
}

// Driver selects the current step each tick.
type Driver struct {
	Kind  DriverKind
	Curve string // amplitude: "linear" (default) or "sqrt"
	Idle  *Idle  // amplitude: optional oscillation when signal <= 0
	FPS   float64
	Mode  string // time: "loop" (default), "pingpong", "once"
}

// Idle is a time oscillation used by an amplitude driver when no signal.
type Idle struct {
	FPS  float64
	Mode string
}

// Steps returns the number of frames the animation produces.
func (d AnimationDef) Steps() int {
	if d.Template != nil {
		return d.Template.Steps
	}
	return len(d.Frames)
}

type rawAnim struct {
	Frames   []string        `json:"frames"`
	Template string          `json:"template"`
	Param    string          `json:"param"`
	From     float64         `json:"from"`
	To       float64         `json:"to"`
	Steps    int             `json:"steps"`
	Driver   json.RawMessage `json:"driver"`
}

type rawDriver struct {
	Type  string `json:"type"`
	Curve string `json:"curve"`
	Idle  *struct {
		FPS  float64 `json:"fps"`
		Mode string  `json:"mode"`
	} `json:"idle"`
	FPS  float64 `json:"fps"`
	Mode string  `json:"mode"`
}

// ParseAnimations parses a map of expression name -> raw animation JSON,
// tolerating per-entry errors: a bad entry is skipped and its error collected.
func ParseAnimations(in map[string]json.RawMessage) (map[string]AnimationDef, []error) {
	out := make(map[string]AnimationDef, len(in))
	var errs []error
	for name, rawDef := range in {
		def, err := parseAnimation(rawDef)
		if err != nil {
			errs = append(errs, fmt.Errorf("animation %q: %w", name, err))
			continue
		}
		out[name] = def
	}
	return out, errs
}

func parseAnimation(data []byte) (AnimationDef, error) {
	var r rawAnim
	if err := json.Unmarshal(data, &r); err != nil {
		return AnimationDef{}, fmt.Errorf("invalid JSON: %w", err)
	}
	hasFrames := len(r.Frames) > 0
	hasTemplate := r.Template != ""
	if hasFrames == hasTemplate {
		return AnimationDef{}, fmt.Errorf("exactly one of frames or template required")
	}

	drv, err := parseDriver(r.Driver)
	if err != nil {
		return AnimationDef{}, err
	}

	if hasFrames {
		for _, n := range r.Frames {
			if !fileNameRe.MatchString(n) {
				return AnimationDef{}, fmt.Errorf("invalid frame name %q", n)
			}
		}
		return AnimationDef{Frames: r.Frames, Driver: drv}, nil
	}

	if !fileNameRe.MatchString(trimSVG(r.Template)) {
		return AnimationDef{}, fmt.Errorf("invalid template file %q", r.Template)
	}
	if r.Param == "" {
		return AnimationDef{}, fmt.Errorf("template requires param")
	}
	if r.Steps < 2 {
		return AnimationDef{}, fmt.Errorf("template requires steps >= 2")
	}
	return AnimationDef{
		Template: &TemplateSource{File: trimSVG(r.Template), Param: r.Param, From: r.From, To: r.To, Steps: r.Steps},
		Driver:   drv,
	}, nil
}

// trimSVG strips a trailing ".svg" so template files may be written with or
// without the extension; the loader always appends ".svg".
func trimSVG(name string) string {
	if len(name) > 4 && name[len(name)-4:] == ".svg" {
		return name[:len(name)-4]
	}
	return name
}

func parseDriver(data []byte) (Driver, error) {
	if len(data) == 0 {
		return Driver{}, fmt.Errorf("missing driver")
	}
	// String shorthand: "amplitude".
	var s string
	if json.Unmarshal(data, &s) == nil {
		if s == string(DriverAmplitude) {
			return Driver{Kind: DriverAmplitude, Curve: "linear"}, nil
		}
		return Driver{}, fmt.Errorf("unknown driver shorthand %q", s)
	}
	var rd rawDriver
	if err := json.Unmarshal(data, &rd); err != nil {
		return Driver{}, fmt.Errorf("invalid driver: %w", err)
	}
	switch DriverKind(rd.Type) {
	case DriverAmplitude:
		curve := rd.Curve
		switch curve {
		case "", "linear":
			curve = "linear"
		case "sqrt":
			// ok
		default:
			return Driver{}, fmt.Errorf("unknown curve %q", curve)
		}
		drv := Driver{Kind: DriverAmplitude, Curve: curve}
		if rd.Idle != nil {
			if rd.Idle.FPS <= 0 {
				return Driver{}, fmt.Errorf("idle requires fps > 0")
			}
			drv.Idle = &Idle{FPS: rd.Idle.FPS, Mode: orMode(rd.Idle.Mode)}
		}
		return drv, nil
	case DriverTime:
		if rd.FPS <= 0 {
			return Driver{}, fmt.Errorf("time driver requires fps > 0")
		}
		mode := orMode(rd.Mode)
		if mode != "loop" && mode != "pingpong" && mode != "once" {
			return Driver{}, fmt.Errorf("unknown mode %q", rd.Mode)
		}
		return Driver{Kind: DriverTime, FPS: rd.FPS, Mode: mode}, nil
	default:
		return Driver{}, fmt.Errorf("unknown driver type %q", rd.Type)
	}
}

func orMode(m string) string {
	if m == "" {
		return "loop"
	}
	return m
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `CGO_ENABLED=0 go test ./internal/face/ -run 'TestParseAnimation|TestParseAnimations' -v`
Expected: PASS

- [ ] **Step 5: Lint and commit**

```bash
golangci-lint run ./internal/face/...
git add internal/face/anim_def.go internal/face/anim_def_test.go
git commit -m "feat(face): animation manifest types and tolerant parser"
```

---

### Task 2: Drivers (time + amplitude step selection)

**Files:**
- Create: `internal/face/anim_driver.go`
- Test: `internal/face/anim_driver_test.go`

- [ ] **Step 1: Write the failing test**

```go
package face

import "testing"

func TestTimeStepLoop(t *testing.T) {
	// fps=10, steps=3 -> indices 0,1,2,0,1,2 at 0.0,0.1,0.2,0.3...
	cases := map[float64]int{0.0: 0, 0.1: 1, 0.25: 2, 0.30: 0, 0.45: 1}
	for clock, want := range cases {
		if got := timeStep(clock, 10, "loop", 3); got != want {
			t.Errorf("timeStep(%v,loop)=%d want %d", clock, got, want)
		}
	}
}

func TestTimeStepPingpong(t *testing.T) {
	// steps=3 -> period 4: 0,1,2,1,0,1,2,1...
	want := []int{0, 1, 2, 1, 0, 1, 2, 1}
	for i, w := range want {
		clock := float64(i) / 10.0
		if got := timeStep(clock, 10, "pingpong", 3); got != w {
			t.Errorf("pingpong i=%d got %d want %d", i, got, w)
		}
	}
}

func TestTimeStepOnceHoldsLast(t *testing.T) {
	if got := timeStep(99.0, 10, "once", 3); got != 2 {
		t.Errorf("once past end = %d want 2", got)
	}
	if got := timeStep(0.1, 10, "once", 3); got != 1 {
		t.Errorf("once mid = %d want 1", got)
	}
}

func TestAmplitudeStepLinearAndSqrt(t *testing.T) {
	lin := Driver{Kind: DriverAmplitude, Curve: "linear"}
	if got := lin.Step(0, 0, 0.5, 6); got != 3 { // round(0.5*5)=3 (0.5*5=2.5 -> +0.5 -> 3)
		t.Errorf("linear 0.5 -> %d want 3", got)
	}
	if got := lin.Step(0, 0, 1.0, 6); got != 5 {
		t.Errorf("linear 1.0 -> %d want 5", got)
	}
	sq := Driver{Kind: DriverAmplitude, Curve: "sqrt"}
	// sqrt(0.25)=0.5 -> round(0.5*5)=3
	if got := sq.Step(0, 0, 0.25, 6); got != 3 {
		t.Errorf("sqrt 0.25 -> %d want 3", got)
	}
}

func TestAmplitudeIdleEngagesAtZeroSignal(t *testing.T) {
	d := Driver{Kind: DriverAmplitude, Curve: "linear", Idle: &Idle{FPS: 10, Mode: "loop"}}
	// signal<=0 -> uses timeStep(clock,10,loop,3)
	if got := d.Step(0.1, 0, 0, 3); got != 1 {
		t.Errorf("idle at clock 0.1 = %d want 1", got)
	}
	// signal>0 -> ignores idle
	if got := d.Step(0.1, 0, 1.0, 3); got != 2 {
		t.Errorf("signal 1.0 = %d want 2", got)
	}
}

func TestStepSingleFrame(t *testing.T) {
	d := Driver{Kind: DriverTime, FPS: 10, Mode: "loop"}
	if got := d.Step(5, 0, 0, 1); got != 0 {
		t.Errorf("single frame = %d want 0", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `CGO_ENABLED=0 go test ./internal/face/ -run 'TestTimeStep|TestAmplitude|TestStepSingle' -v`
Expected: FAIL — `undefined: timeStep` and `Step`.

- [ ] **Step 3: Write the implementation**

```go
package face

import "math"

// Step returns the current frame index in [0, steps) for this driver.
//   clock  — absolute seconds (loop/pingpong, and amplitude idle)
//   epoch  — seconds since the expression became active (time "once")
//   signal — amplitude in [0,1]
func (d Driver) Step(clock, epoch float64, signal float32, steps int) int {
	if steps <= 1 {
		return 0
	}
	switch d.Kind {
	case DriverAmplitude:
		if signal <= 0 && d.Idle != nil {
			return timeStep(clock, d.Idle.FPS, d.Idle.Mode, steps)
		}
		v := float64(signal)
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		if d.Curve == "sqrt" {
			v = math.Sqrt(v)
		}
		return clampStep(int(v*float64(steps-1)+0.5), steps)
	case DriverTime:
		elapsed := clock
		if d.Mode == "once" {
			elapsed = clock - epoch
			if elapsed < 0 {
				elapsed = 0
			}
		}
		return timeStep(elapsed, d.FPS, d.Mode, steps)
	default:
		return 0
	}
}

// timeStep maps elapsed seconds to a frame index given fps, mode and steps.
func timeStep(elapsed, fps float64, mode string, steps int) int {
	if steps <= 1 {
		return 0
	}
	idx := int(elapsed * fps)
	if idx < 0 {
		idx = 0
	}
	switch mode {
	case "pingpong":
		period := 2 * (steps - 1)
		p := idx % period
		if p < steps {
			return p
		}
		return period - p
	case "once":
		if idx >= steps {
			return steps - 1
		}
		return idx
	default: // loop
		return idx % steps
	}
}

func clampStep(s, steps int) int {
	if s < 0 {
		return 0
	}
	if s >= steps {
		return steps - 1
	}
	return s
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `CGO_ENABLED=0 go test ./internal/face/ -run 'TestTimeStep|TestAmplitude|TestStepSingle' -v`
Expected: PASS

- [ ] **Step 5: Lint and commit**

```bash
golangci-lint run ./internal/face/...
git add internal/face/anim_driver.go internal/face/anim_driver_test.go
git commit -m "feat(face): time and amplitude animation drivers"
```

---

### Task 3: Frame providers + literal-name loader

**Files:**
- Create: `internal/face/anim_frames.go`
- Modify: `internal/face/library.go` (add `rawBytes`)
- Test: `internal/face/anim_frames_test.go`

- [ ] **Step 1: Write the failing test**

```go
package face

import (
	"os"
	"path/filepath"
	"testing"
)

const tinyRedSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><rect x="0" y="0" width="10" height="10" fill="#ff0000"/></svg>`

func writeSVG(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name+".svg"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBuildFramesList(t *testing.T) {
	dir := t.TempDir()
	writeSVG(t, dir, "f0", tinyRedSVG)
	writeSVG(t, dir, "f1", tinyRedSVG)
	lib := NewLibraryMode(dir, true) // self-contained: only on-disk frames
	def := AnimationDef{Frames: []string{"f0", "f1"}, Driver: Driver{Kind: DriverAmplitude, Curve: "linear"}}
	frames, err := buildFrames(lib, def, 4, 4)
	if err != nil {
		t.Fatalf("buildFrames: %v", err)
	}
	if len(frames) != 2 || len(frames[0]) != 16 {
		t.Fatalf("frames=%d size=%d", len(frames), len(frames[0]))
	}
}

func TestBuildFramesTemplate(t *testing.T) {
	dir := t.TempDir()
	// width grows with V so steps differ
	tmpl := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><rect x="0" y="0" width="{{.V}}" height="10" fill="#0000ff"/></svg>`
	writeSVG(t, dir, "bar", tmpl)
	lib := NewLibraryMode(dir, true)
	def := AnimationDef{
		Template: &TemplateSource{File: "bar", Param: "V", From: 1, To: 10, Steps: 3},
		Driver:   Driver{Kind: DriverTime, FPS: 4, Mode: "loop"},
	}
	frames, err := buildFrames(lib, def, 8, 8)
	if err != nil {
		t.Fatalf("buildFrames: %v", err)
	}
	if len(frames) != 3 {
		t.Fatalf("frames=%d want 3", len(frames))
	}
}

func TestRawBytesPrefersOverrideThenEmbedded(t *testing.T) {
	dir := t.TempDir()
	writeSVG(t, dir, "speaking_0", tinyRedSVG)
	lib := NewLibrary(dir) // overlay: embedded fallback enabled
	data, ok := lib.rawBytes("speaking_0")
	if !ok || len(data) == 0 {
		t.Fatal("override speaking_0 not found")
	}
	// neutral is embedded; rawBytes finds it by literal name with no override.
	if d, ok := NewLibrary(t.TempDir()).rawBytes("neutral"); !ok || len(d) == 0 {
		t.Fatal("embedded neutral not found by rawBytes")
	}
}

func TestBuildFramesMissingFrameErrors(t *testing.T) {
	lib := NewLibraryMode(t.TempDir(), true)
	def := AnimationDef{Frames: []string{"nope"}, Driver: Driver{Kind: DriverAmplitude}}
	if _, err := buildFrames(lib, def, 4, 4); err == nil {
		t.Fatal("expected error for missing frame")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `CGO_ENABLED=0 go test ./internal/face/ -run 'TestBuildFrames|TestRawBytes' -v`
Expected: FAIL — `undefined: buildFrames`, `lib.rawBytes`.

- [ ] **Step 3a: Add `rawBytes` to `internal/face/library.go`**

Append this method to `library.go` (after `Resolve`):

```go
// rawBytes returns SVG bytes for a literal (non-canonicalized) name: a
// faces/<name>.svg override if present, else the embedded assets/<name>.svg.
// Used by the animation engine, where frame and template names are literal
// asset names rather than expression aliases. Self-contained libraries do not
// fall back to embedded assets.
func (l *Library) rawBytes(name string) ([]byte, bool) {
	if !fileNameRe.MatchString(name) {
		return nil, false
	}
	if l.dir != "" {
		if data, err := os.ReadFile(filepath.Join(l.dir, name+".svg")); err == nil && len(bytes.TrimSpace(data)) > 0 {
			return data, true
		}
	}
	if l.selfContained {
		return nil, false
	}
	return defaultBytes(name)
}
```

- [ ] **Step 3b: Write `internal/face/anim_frames.go`**

```go
package face

import (
	"bytes"
	"fmt"
	"text/template"
)

// buildFrames rasterizes every step of def into a full w×h ARGB buffer.
func buildFrames(lib *Library, def AnimationDef, w, h int) ([][]uint32, error) {
	n := def.Steps()
	if n < 1 {
		return nil, fmt.Errorf("animation has no steps")
	}
	out := make([][]uint32, n)
	for i := 0; i < n; i++ {
		svg, err := frameSVG(lib, def, i, n)
		if err != nil {
			return nil, err
		}
		buf, err := Rasterize(svg, w, h)
		if err != nil {
			return nil, fmt.Errorf("rasterize step %d: %w", i, err)
		}
		out[i] = buf
	}
	return out, nil
}

// frameSVG returns the SVG bytes for step i of an n-step animation.
func frameSVG(lib *Library, def AnimationDef, i, n int) ([]byte, error) {
	if def.Template != nil {
		data, ok := lib.rawBytes(def.Template.File)
		if !ok {
			return nil, fmt.Errorf("template %q not found", def.Template.File)
		}
		val := def.Template.From
		if n > 1 {
			val = def.Template.From + (def.Template.To-def.Template.From)*float64(i)/float64(n-1)
		}
		return renderAnimTemplate(data, def.Template.Param, val)
	}
	name := def.Frames[i]
	data, ok := lib.rawBytes(name)
	if !ok {
		return nil, fmt.Errorf("frame %q not found", name)
	}
	return data, nil
}

// renderAnimTemplate executes a Go-template SVG with a single named parameter.
func renderAnimTemplate(data []byte, param string, val float64) ([]byte, error) {
	tmpl, err := template.New("anim").Parse(string(data))
	if err != nil {
		return nil, fmt.Errorf("parse animation template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]any{param: val}); err != nil {
		return nil, fmt.Errorf("execute animation template: %w", err)
	}
	return buf.Bytes(), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `CGO_ENABLED=0 go test ./internal/face/ -run 'TestBuildFrames|TestRawBytes' -v`
Expected: PASS

- [ ] **Step 5: Lint and commit**

```bash
golangci-lint run ./internal/face/...
git add internal/face/anim_frames.go internal/face/library.go internal/face/anim_frames_test.go
git commit -m "feat(face): animation frame providers and literal-name loader"
```

---

### Task 4: Engine with async, single-flight, scoped cache

**Files:**
- Create: `internal/face/anim_engine.go`
- Test: `internal/face/anim_engine_test.go`

**Context:** The engine must never block the render loop. Rasterizing N full frames can take ~1s on the device, so a build runs on a background goroutine; until it finishes, `AnimFrame` returns `(nil,false)` and the renderer shows the static face. Only one expression's frames are resident; switching expression supersedes any in-flight build.

- [ ] **Step 1: Write the failing test**

```go
package face

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	dir := t.TempDir()
	for _, n := range []string{"talk_0", "talk_1"} {
		if err := os.WriteFile(filepath.Join(dir, n+".svg"), []byte(tinyRedSVG), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	lib := NewLibraryMode(dir, true)
	defs := map[string]AnimationDef{
		"talk": {Frames: []string{"talk_0", "talk_1"}, Driver: Driver{Kind: DriverAmplitude, Curve: "linear"}},
	}
	return NewEngine(lib, defs)
}

func waitReady(t *testing.T, e *Engine, expr string, w, h int) []uint32 {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if buf, ok := e.AnimFrame(expr, w, h, 0, 0, 1.0); ok {
			return buf
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("animation %q never became ready", expr)
	return nil
}

func TestEngineHasOnlyAnimated(t *testing.T) {
	e := newTestEngine(t)
	if !e.Has("talk") {
		t.Fatal("expected Has(talk)")
	}
	if e.Has("neutral") {
		t.Fatal("neutral is not animated")
	}
}

func TestEngineReturnsFalseForStatic(t *testing.T) {
	e := newTestEngine(t)
	if _, ok := e.AnimFrame("neutral", 4, 4, 0, 0, 0); ok {
		t.Fatal("static expr should return false")
	}
}

func TestEngineBuildsAndServesFrames(t *testing.T) {
	e := newTestEngine(t)
	buf := waitReady(t, e, "talk", 4, 4)
	if len(buf) != 16 {
		t.Fatalf("frame size=%d want 16", len(buf))
	}
}

func TestEngineConcurrentAccessRaceClean(t *testing.T) {
	e := newTestEngine(t)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(k int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				expr := "talk"
				if k%2 == 0 {
					expr = "neutral"
				}
				e.AnimFrame(expr, 4, 4, float64(j)/10, 0, float32(j%2))
			}
		}(i)
	}
	wg.Wait()
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `CGO_ENABLED=0 go test ./internal/face/ -run TestEngine -v`
Expected: FAIL — `undefined: NewEngine`.

- [ ] **Step 3: Write the implementation**

```go
package face

import (
	"strings"
	"sync"
)

// Engine renders declarative animations. Only the active expression's frames
// are resident; builds run on a background goroutine so the render loop never
// blocks. AnimFrame returns (nil,false) until the active animation is ready.
type Engine struct {
	lib  *Library
	defs map[string]AnimationDef

	mu       sync.Mutex
	expr     string // resident/in-flight expression key
	w, h     int
	ready    bool
	building bool
	frames   [][]uint32
}

// NewEngine returns an Engine over lib with the given effective animation set
// (keyed by lowercase expression name).
func NewEngine(lib *Library, defs map[string]AnimationDef) *Engine {
	return &Engine{lib: lib, defs: defs}
}

// Has reports whether expr has a declared animation.
func (e *Engine) Has(expr string) bool {
	_, ok := e.defs[normExpr(expr)]
	return ok
}

// Prewarm asynchronously builds expr's frames at w×h so the first display is
// smooth. Safe to call from any goroutine; a no-op for static expressions.
func (e *Engine) Prewarm(expr string, w, h int) {
	key := normExpr(expr)
	def, ok := e.defs[key]
	if !ok {
		return
	}
	e.mu.Lock()
	e.ensureLocked(key, def, w, h)
	e.mu.Unlock()
}

// AnimFrame returns the current frame for expr at w×h, or (nil,false) when expr
// is static or its animation is not yet built.
func (e *Engine) AnimFrame(expr string, w, h int, clock, epoch float64, signal float32) ([]uint32, bool) {
	key := normExpr(expr)
	def, ok := e.defs[key]
	if !ok {
		return nil, false
	}
	e.mu.Lock()
	e.ensureLocked(key, def, w, h)
	if !e.ready || e.expr != key || e.w != w || e.h != h {
		e.mu.Unlock()
		return nil, false
	}
	frames := e.frames
	e.mu.Unlock()

	step := def.Driver.Step(clock, epoch, signal, len(frames))
	if step < 0 || step >= len(frames) {
		return nil, false
	}
	return frames[step], true
}

// ensureLocked starts a background build if the resident state does not match
// (key,w,h) and no matching build is already in flight. Caller holds e.mu.
func (e *Engine) ensureLocked(key string, def AnimationDef, w, h int) {
	if e.expr == key && e.w == w && e.h == h && (e.ready || e.building) {
		return
	}
	e.expr, e.w, e.h = key, w, h
	e.ready, e.building, e.frames = false, true, nil
	lib := e.lib
	go func() {
		frames, err := buildFrames(lib, def, w, h)
		e.mu.Lock()
		defer e.mu.Unlock()
		if e.expr != key || e.w != w || e.h != h {
			return // superseded by a newer request
		}
		e.building = false
		if err == nil {
			e.frames = frames
			e.ready = true
		}
	}()
}

func normExpr(expr string) string {
	return strings.ToLower(strings.TrimSpace(expr))
}
```

- [ ] **Step 4: Run tests (with the race detector)**

Run: `CGO_ENABLED=1 go test -race ./internal/face/ -run TestEngine -v`
Expected: PASS, no race warnings.
Also run: `CGO_ENABLED=0 go test ./internal/face/ -run TestEngine -v`
Expected: PASS

- [ ] **Step 5: Lint and commit**

```bash
golangci-lint run ./internal/face/...
git add internal/face/anim_engine.go internal/face/anim_engine_test.go
git commit -m "feat(face): async scoped animation engine"
```

---

### Task 5: Generate speaking frames + embedded default animation

**Files:**
- Create: `cmd/gen-speaking-frames/main.go`
- Create (generated, committed): `internal/face/assets/speaking_0.svg` … `speaking_5.svg`
- Replace: `internal/face/assets/speaking.svg` (now a static closed-mouth SVG = step 0)
- Create: `internal/face/anim_defaults.go`
- Test: `internal/face/anim_defaults_test.go`

**Context:** Today `assets/speaking.svg` is a Go *template*; rasterizing it directly fails. We replace it with a concrete closed-mouth SVG (used as the static fallback) and add six concrete frames. The generator carries its own copy of the mouth geometry (moved out of the soon-to-be-deleted `speak.go`).

- [ ] **Step 1: Write the generator `cmd/gen-speaking-frames/main.go`**

```go
// Command gen-speaking-frames writes the BMO speaking animation assets:
// speaking_0.svg (closed) .. speaking_5.svg (open), plus speaking.svg (= step 0,
// the static fallback). Run from the repo root:
//
//	go run ./cmd/gen-speaking-frames
//
//go:generate go run ../gen-speaking-frames
package main

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"text/template"
)

const speakLevels = 6

const speakTemplate = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">
  <rect x="0" y="0" width="280" height="210" fill="#4ECBA8"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#4ECBA8"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#90e5c8"/>
  <rect x="76"  y="68" width="7" height="20" rx="3" ry="3" fill="#1a1a1a"/>
  <rect x="195" y="68" width="7" height="20" rx="3" ry="3" fill="#1a1a1a"/>
  <rect x="106" y="106" width="68" height="{{.MouthH}}" rx="{{.MouthRx}}" ry="{{.MouthRx}}" fill="#1a1a1a"/>
  <path d="{{.TeethPath}}" fill="#e4e4e4"/>
  <path d="{{.InteriorPath}}" fill="#1a7848"/>
  <path d="{{.TonguePath}}" fill="#16ae81"/>
</svg>
`

type speakParams struct {
	MouthH       float64
	MouthRx      float64
	TeethPath    string
	InteriorPath string
	TonguePath   string
}

func computeParams(t float64) speakParams {
	t = math.Max(0, math.Min(1, t))
	const left, right, top = 106.0, 174.0, 106.0
	h := 6 + 30*t
	r := math.Min(16, h/2)
	bottom := top + h

	th := 0.28 * h
	dy := r - th
	dx := math.Sqrt(r*r - dy*dy)
	tlx, trx := left+r-dx, right-r+dx
	tby := top + th

	teeth := fmt.Sprintf(
		"M %.2f %.2f A %.2f %.2f 0 0 1 %.2f %.2f L %.2f %.2f A %.2f %.2f 0 0 1 %.2f %.2f Z",
		tlx, tby, r, r, left+r, top, right-r, top, r, r, trx, tby)

	interior := fmt.Sprintf(
		"M %.2f %.2f L %.2f %.2f "+
			"A %.2f %.2f 0 0 1 %.2f %.2f L %.2f %.2f "+
			"A %.2f %.2f 0 0 1 %.2f %.2f L %.2f %.2f "+
			"A %.2f %.2f 0 0 1 %.2f %.2f L %.2f %.2f "+
			"A %.2f %.2f 0 0 1 %.2f %.2f Z",
		tlx, tby, trx, tby,
		r, r, right, top+r, right, bottom-r,
		r, r, right-r, bottom, left+r, bottom,
		r, r, left, bottom-r, left, top+r,
		r, r, tlx, tby)

	tr := 19.0 * h / 36.0
	ty := 0.18 * h
	tongue := fmt.Sprintf("M %.2f %.2f Q %.2f %.2f %.2f %.2f Z",
		140-tr, bottom, 140.0, bottom-2*ty, 140+tr, bottom)

	return speakParams{MouthH: h, MouthRx: r, TeethPath: teeth, InteriorPath: interior, TonguePath: tongue}
}

func main() {
	outDir := filepath.Join("internal", "face", "assets")
	tmpl := template.Must(template.New("speak").Parse(speakTemplate))
	for lvl := 0; lvl < speakLevels; lvl++ {
		t := float64(lvl) / float64(speakLevels-1)
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, computeParams(t)); err != nil {
			panic(err)
		}
		name := fmt.Sprintf("speaking_%d.svg", lvl)
		if err := os.WriteFile(filepath.Join(outDir, name), buf.Bytes(), 0o644); err != nil {
			panic(err)
		}
		if lvl == 0 {
			// Static fallback = closed mouth.
			if err := os.WriteFile(filepath.Join(outDir, "speaking.svg"), buf.Bytes(), 0o644); err != nil {
				panic(err)
			}
		}
		fmt.Println("wrote", name)
	}
}
```

- [ ] **Step 2: Run the generator and verify assets exist**

Run: `go run ./cmd/gen-speaking-frames && ls internal/face/assets/speaking*.svg`
Expected: lists `speaking.svg`, `speaking_0.svg` … `speaking_5.svg`, none containing `{{`.
Verify no template markers remain:
Run: `grep -L '{{' internal/face/assets/speaking_*.svg | wc -l`
Expected: `6`

- [ ] **Step 3: Write `internal/face/anim_defaults.go`**

```go
package face

// DefaultAnimations returns the built-in animation set baked into the binary.
// Overlay mods inherit these; self-contained mods do not (they declare their
// own). The speaking mouth is six frames driven by lip-sync amplitude, with a
// gentle idle oscillation (~1.3 Hz, matching the previous sine fallback) when
// amplitude is unavailable.
func DefaultAnimations() map[string]AnimationDef {
	return map[string]AnimationDef{
		ExprSpeaking: {
			Frames: []string{"speaking_0", "speaking_1", "speaking_2", "speaking_3", "speaking_4", "speaking_5"},
			Driver: Driver{
				Kind:  DriverAmplitude,
				Curve: "sqrt",
				Idle:  &Idle{FPS: 13, Mode: "pingpong"},
			},
		},
	}
}
```

- [ ] **Step 4: Write `internal/face/anim_defaults_test.go`**

```go
package face

import "testing"

func TestDefaultSpeakingAnimation(t *testing.T) {
	defs := DefaultAnimations()
	sp, ok := defs[ExprSpeaking]
	if !ok {
		t.Fatal("speaking default missing")
	}
	if sp.Steps() != 6 {
		t.Fatalf("speaking steps=%d want 6", sp.Steps())
	}
	if sp.Driver.Kind != DriverAmplitude || sp.Driver.Curve != "sqrt" || sp.Driver.Idle == nil {
		t.Fatalf("driver=%+v", sp.Driver)
	}
}

func TestDefaultSpeakingFramesRasterizeAndDiffer(t *testing.T) {
	lib := NewLibrary(t.TempDir()) // overlay: embedded assets only
	frames, err := buildFrames(lib, DefaultAnimations()[ExprSpeaking], 80, 60)
	if err != nil {
		t.Fatalf("buildFrames: %v", err)
	}
	if len(frames) != 6 {
		t.Fatalf("frames=%d want 6", len(frames))
	}
	// Every frame is a full buffer (Rasterize already rejects blank frames).
	for i, f := range frames {
		if len(f) != 80*60 {
			t.Fatalf("frame %d size=%d want %d", i, len(f), 80*60)
		}
	}
	// Closed (step 0) and fully-open (step 5) mouths must differ.
	if equalFrame(frames[0], frames[5]) {
		t.Fatal("closed and open speaking frames are identical")
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

- [ ] **Step 5: Run tests**

Run: `CGO_ENABLED=0 go test ./internal/face/ -run 'TestDefaultSpeaking' -v`
Expected: PASS

- [ ] **Step 6: Lint and commit**

```bash
golangci-lint run ./cmd/gen-speaking-frames/... ./internal/face/...
git add cmd/gen-speaking-frames/main.go internal/face/anim_defaults.go internal/face/anim_defaults_test.go internal/face/assets/speaking.svg internal/face/assets/speaking_0.svg internal/face/assets/speaking_1.svg internal/face/assets/speaking_2.svg internal/face/assets/speaking_3.svg internal/face/assets/speaking_4.svg internal/face/assets/speaking_5.svg
git commit -m "feat(face): generate speaking frames and embedded default animation"
```

---

### Task 6: Manifest `animations` field

**Files:**
- Modify: `internal/mod/manifest.go`
- Test: `internal/mod/manifest_test.go`

- [ ] **Step 1: Write the failing test** (append to `manifest_test.go`)

```go
func TestLoadManifestAnimations(t *testing.T) {
	dir := t.TempDir()
	body := `{"name":"Evil","animations":{"speaking":{"frames":["m0","m1"],"driver":"amplitude"}}}`
	if err := os.WriteFile(filepath.Join(dir, "mod.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	m := LoadManifest(dir)
	if _, ok := m.Animations["speaking"]; !ok {
		t.Fatal("speaking animation not parsed")
	}
}

func TestLoadManifestNoAnimations(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "mod.json"), []byte(`{"name":"x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if m := LoadManifest(dir); m.Animations != nil {
		t.Fatalf("expected nil animations, got %v", m.Animations)
	}
}
```

Ensure `manifest_test.go` imports `os`, `path/filepath`, `testing` (add any missing).

- [ ] **Step 2: Run test to verify it fails**

Run: `CGO_ENABLED=0 go test ./internal/mod/ -run TestLoadManifestAnimations -v`
Expected: FAIL — `m.Animations undefined`.

- [ ] **Step 3: Add the field**

`internal/mod/manifest.go` already imports `encoding/json` (used by `LoadManifest`), so no import change is needed. Add this field to `Manifest` after `Emotions`:

```go
	// Animations maps an expression name to its raw animation JSON. Parsing of
	// the inner shape is deferred to internal/face (ParseAnimations) so this
	// package stays free of rendering concerns and tolerant of unknown fields.
	Animations map[string]json.RawMessage `json:"animations,omitempty"`
```

- [ ] **Step 4: Run test to verify it passes**

Run: `CGO_ENABLED=0 go test ./internal/mod/ -run TestLoadManifest -v`
Expected: PASS

- [ ] **Step 5: Lint and commit**

```bash
golangci-lint run ./internal/mod/...
git add internal/mod/manifest.go internal/mod/manifest_test.go
git commit -m "feat(mod): raw animations field in manifest"
```

---

### Task 7: Renderer integration

**Files:**
- Modify: `internal/renderer/bmo.go`
- Test: `internal/renderer/anim_epoch_test.go`

**Context:** Replace the `speaking` special-case in `blitFace` with an engine lookup, and track when the active expression became current (for `once`). Extract the epoch bookkeeping into a pure helper so it is testable without SDL.

- [ ] **Step 1: Write the failing test (pure, no SDL)**

```go
package renderer

import "testing"

func TestExprEpochResetsOnChange(t *testing.T) {
	var tr exprTracker
	// First observation of "neutral" at t=10 sets the start.
	if got := tr.epoch("neutral", 10.0); got != 0 {
		t.Fatalf("initial epoch=%v want 0", got)
	}
	// Same expr later -> elapsed since start.
	if got := tr.epoch("neutral", 13.5); got != 3.5 {
		t.Fatalf("epoch=%v want 3.5", got)
	}
	// Change expr -> resets.
	if got := tr.epoch("speaking", 14.0); got != 0 {
		t.Fatalf("epoch after change=%v want 0", got)
	}
	if got := tr.epoch("speaking", 16.0); got != 2.0 {
		t.Fatalf("epoch=%v want 2.0", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/renderer/ -run TestExprEpoch -v`
Expected: FAIL — `undefined: exprTracker`.

- [ ] **Step 3: Add the tracker, engine field, and rewire `blitFace`**

In `internal/renderer/bmo.go`:

3a. Add the tracker type (near `FrameState`):

```go
// exprTracker remembers when the current expression became active so time
// "once" animations can measure elapsed seconds since the cut.
type exprTracker struct {
	expr  string
	start float64
}

// epoch returns seconds elapsed since expr became the active expression,
// resetting to 0 the first tick a new expression appears.
func (e *exprTracker) epoch(expr string, clock float64) float64 {
	if expr != e.expr {
		e.expr = expr
		e.start = clock
		return 0
	}
	return clock - e.start
}
```

3b. Add fields to `Renderer`:

```go
	anims   *face.Engine
	exprTr  exprTracker
```

3c. Add the setter (next to `SetFaces`):

```go
// SetAnimations installs the declarative animation engine. Call before the
// render loop and on mod switch.
func (r *Renderer) SetAnimations(e *face.Engine) {
	r.anims = e
}
```

3d. Replace the body of `blitFace` (the whole `if face.Canonical(expr) == face.ExprSpeaking { ... }` block plus the trailing static path) with:

```go
func (r *Renderer) blitFace(expr string, frame FrameState, phase float64) bool {
	if r.anims != nil {
		epoch := r.exprTr.epoch(expr, phase)
		if buf, ok := r.anims.AnimFrame(expr, int(r.W), int(r.H), phase, epoch, frame.SpeakAmplitude); ok {
			if len(buf) != len(r.pixels) {
				return false
			}
			copy(r.pixels, buf)
			return true
		}
	}
	if r.faces == nil {
		return false
	}
	buf := r.faces.Frame(expr, int(r.W), int(r.H))
	if buf == nil || len(buf) != len(r.pixels) {
		return false
	}
	copy(r.pixels, buf)
	return true
}
```

3e. Remove the now-unused `blitStrip` method and the `math` import **only if** no longer referenced elsewhere in the file (the corner clock uses `math`, so keep the import). Remove `blitStrip` entirely.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/renderer/ -run TestExprEpoch -v`
Expected: PASS
Run: `CGO_ENABLED=1 go build ./internal/renderer/`
Expected: builds (no unused-symbol errors). If `blitStrip` removal left `Strip` references, they are removed in Task 8.

- [ ] **Step 5: Lint and commit**

```bash
golangci-lint run ./internal/renderer/...
git add internal/renderer/bmo.go internal/renderer/anim_epoch_test.go
git commit -m "feat(renderer): drive faces through the animation engine"
```

---

### Task 8: Delete the bespoke speaking machinery

**Files:**
- Delete: `internal/face/speak.go` (and `internal/face/speak_test.go` if present)
- Modify: `internal/face/cache.go`
- Possibly modify: any file referencing deleted symbols

**Context:** With the engine live and `speaking` served as a default animation, the old strip/template path is dead. Remove it and prove nothing references it.

- [ ] **Step 1: Find all references to the symbols being deleted**

Run:
```bash
grep -rn 'Speak\|speakSet\|\bStrip\b\|speakBand\|speakParams\|renderSpeakSVG\|renderSpeakLevels\|IsSpeakTemplate\|SpeakReady\|speakLevels\|warmSpeak\|buildSpeak' --include=*.go internal cmd | grep -v cmd/gen-speaking-frames
```
Expected after Task 7: references only inside `internal/face/cache.go`, `internal/face/speak.go`, and their tests (the renderer no longer references `Speak`/`Strip`). Note each call site to update.

- [ ] **Step 2: Delete `speak.go` and remove speaking members from `cache.go`**

```bash
git rm internal/face/speak.go
# If a dedicated test exists:
test -f internal/face/speak_test.go && git rm internal/face/speak_test.go || true
```

In `internal/face/cache.go` remove:
- the `Strip` and `speakSet` type declarations;
- the `speak *speakSet` and `speakFailed bool` fields from `Cache`;
- the methods `Speak`, `SpeakReady`, `warmSpeak`, `buildSpeakLocked`, `buildSpeakFromDefault`, and `renderSpeakLevels`;
- the `c.warmSpeak(w, h)` call inside `Warm`;
- the `c.speak = nil` and `c.speakFailed = false` lines inside `resizeLocked`.

Leave `Frame`, `warmFrame`, `renderLocked`, `resizeLocked` (minus the speak lines), `resolved`, and the canonical/disk warm loops intact.

- [ ] **Step 3: Verify the package builds and tests pass**

Run: `CGO_ENABLED=0 go build ./internal/face/`
Expected: builds with no undefined symbols.
Run: `CGO_ENABLED=0 go test ./internal/face/`
Expected: PASS (remove or update any leftover test that referenced deleted symbols; there should be none after Step 1).

- [ ] **Step 4: Verify the renderer still builds (CGO)**

Run: `CGO_ENABLED=1 go build ./internal/renderer/ ./cmd/bmo-pak/`
Expected: builds. If `cmd/bmo-pak` references `SpeakReady`/`Speak`, fix in Task 9 (it does not today).

- [ ] **Step 5: Lint and commit**

```bash
golangci-lint run ./internal/face/... ./internal/renderer/...
git add -A internal/face/ internal/renderer/
git commit -m "refactor(face): remove bespoke speaking animation path"
```

---

### Task 9: Wire the engine in `cmd/bmo-pak`

**Files:**
- Modify: `cmd/bmo-pak/main.go`

**Context:** Build the effective animation set (embedded defaults unless self-contained, overlaid with the mod's parsed `animations`, mod winning), install the engine at startup and on mod switch, and prewarm `speaking` so the first utterance animates immediately.

- [ ] **Step 1: Add a helper to build the engine**

Add this function near the other small helpers in `cmd/bmo-pak/main.go`:

```go
// buildAnimationEngine assembles the effective animation set for the active mod
// and returns an engine over lib. Overlay mods inherit the embedded defaults;
// self-contained mods own their set. Mod-declared animations win by name.
// Parse errors are logged and the offending entry is skipped.
func buildAnimationEngine(lib *face.Library, m mod.Mod, logf func(string, ...any)) *face.Engine {
	defs := map[string]face.AnimationDef{}
	if !m.SelfContained() {
		for k, v := range face.DefaultAnimations() {
			defs[k] = v
		}
	}
	modDefs, errs := face.ParseAnimations(m.Manifest.Animations)
	for _, e := range errs {
		logf("face: %v", e)
	}
	for k, v := range modDefs {
		defs[k] = v
	}
	return face.NewEngine(lib, defs)
}
```

- [ ] **Step 2: Install the engine at startup**

After the existing startup block (`faceCache := face.NewCache(faceLib)` … `go faceCache.Warm(screen.Size())`, around line 283–286), add:

```go
	animEngine := buildAnimationEngine(faceLib, activeMod, logger.Warnf)
	screen.SetAnimations(animEngine)
	{
		w, h := screen.Size()
		go animEngine.Prewarm(face.ExprSpeaking, w, h)
	}
```

- [ ] **Step 3: Rebuild the engine on mod switch**

Inside `reloadMod`, after `go newCache.Warm(screen.Size())` (around line 305), add:

```go
		newEngine := buildAnimationEngine(newLib, active, logger.Warnf)
		screen.SetAnimations(newEngine)
		{
			w, h := screen.Size()
			go newEngine.Prewarm(face.ExprSpeaking, w, h)
		}
```

- [ ] **Step 4: Build**

Run: `CGO_ENABLED=1 go build ./cmd/bmo-pak/`
Expected: builds. Confirm `face` and `mod` are already imported in `main.go` (they are).

- [ ] **Step 5: Lint and commit**

```bash
golangci-lint run ./cmd/bmo-pak/...
git add cmd/bmo-pak/main.go
git commit -m "feat(bmo-pak): install declarative animation engine with live reload"
```

---

### Task 10: Full verification

**Files:** none (verification only)

- [ ] **Step 1: Whole-module build, both CGO modes**

Run: `CGO_ENABLED=0 go build ./... && CGO_ENABLED=1 go build ./...`
Expected: both succeed.

- [ ] **Step 2: Full test suite**

Run: `CGO_ENABLED=0 go test ./...`
Expected: PASS (only `[no test files]` notices for packages without tests).

- [ ] **Step 3: Race check on the animation engine**

Run: `CGO_ENABLED=1 go test -race ./internal/face/ ./internal/renderer/`
Expected: PASS, no race warnings.

- [ ] **Step 4: Lint the whole module**

Run: `golangci-lint run ./...`
Expected: 0 issues.

- [ ] **Step 5: Confirm the dead path is gone**

Run:
```bash
grep -rn 'Cache) Speak\|speakSet\|renderSpeakLevels' --include=*.go internal | grep -v cmd/gen-speaking-frames
```
Expected: no output.

- [ ] **Step 6: Commit any final touch-ups** (only if Steps 1–5 required edits)

```bash
git add -A
git commit -m "test(face): finalize animation engine verification"
```

---

## Self-Review Notes (for the executor)

- **Spec coverage:** hybrid frame model (Task 3), amplitude+time drivers with swappable signal as a `float32` input (Tasks 2, 7), full-frames + scoped async warming (Task 4), `mod.json` schema (Tasks 1, 6), merge rule incl. self-contained (Task 9), dogfood speaking + deletion (Tasks 5, 7, 8), all-three time modes (Task 2).
- **Signal interface:** the spec's `Signal interface { Value() float32 }` lives at the wiring layer; in this plan the engine consumes the `float32` directly (`frame.SpeakAmplitude`), so a future viseme source only changes what populates that field — no engine/schema change. No separate interface type is introduced now (YAGNI); add it when a second signal ships.
- **Naming consistency:** `AnimFrame(expr, w, h, clock, epoch, signal)`, `DriverKind`/`DriverAmplitude`/`DriverTime`, `AnimationDef.Steps()`, `Library.rawBytes`, `Engine.Prewarm`, `DefaultAnimations()` are used identically across tasks.
- **Idle parity:** `Idle{FPS:13, Mode:"pingpong"}` approximates the previous `0.45+0.35·sin(phase·8)` (~1.27 Hz) fallback; exact pixel parity is not required (spec: "approximate").
