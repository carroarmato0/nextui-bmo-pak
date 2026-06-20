# Hey BMO Wake-Word Model Training — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the tooling and documented, reproducible pipeline that produces BMO's own "Hey BMO" (pronounced "Beemo") wake-word classifier, then train, validate, commit, and ship it in place of the stock `hey_jarvis` placeholder.

**Architecture:** A small Go evaluation tool (`cmd/wakeword-eval`) scores a candidate `.onnx` against the existing on-device detector (`internal/wakeword`) over folders of WAVs and checks the model contract. A pinned, Colab/local-GPU notebook (`training/wakeword/`) drives openWakeWord's tooling to synthesize, augment, and train the classifier from a single editable `config.yaml`. The trained `hey_bmo.onnx` is committed to `assets/wakeword/` and the fetch script stops downloading the placeholder.

**Tech Stack:** Go 1.25 + cgo (`github.com/yalue/onnxruntime_go` v1.31.0, ONNX Runtime 1.26.0), openWakeWord v0.5.1, piper-sample-generator, Python (Colab free GPU or local NVIDIA GPU), bash release scripts.

**Reference spec:** `docs/superpowers/specs/2026-06-19-hey-bmo-model-training-design.md`

---

## Background the engineer needs

- **The detector already exists** (`internal/wakeword/detector.go`). It runs three ONNX models in series: a shared **melspectrogram** model → a shared **embedding** model → the wake-word **classifier**. Only the classifier is phrase-specific. We are producing a new classifier; the base models stay fixed (openWakeWord v0.5.1).
- **Classifier I/O contract:** input float32 `[1, 16, 96]` (16 embeddings ≈ 1.28 s), output float32 `[1, 1]` sigmoid score. Internal constants in `detector.go`: `classWindow = 16`, `embDim = 96`.
- **The detector API** (all on `*wakeword.Detector`):
  - `wakeword.New(wakeword.Config) (*Detector, error)` — `Config{LibraryPath, MelModel, EmbModel, WakeModel string; Threshold float64; Threads, RefractorySteps int}`. `Threshold<=0`→0.5, `Threads<=0`→2, `RefractorySteps<=0`→12. `New` initializes the process-global ORT env via the shared library at `LibraryPath`.
  - `(*Detector).Write(pcm []byte) []Detection` — appends S16LE mono 16 kHz PCM and returns a `Detection{Score float64}` for each 80 ms hop (1280 samples = 2560 bytes) where the score crosses `Threshold` and the refractory gate is clear.
  - `(*Detector).Reset()` — clears streaming context and re-arms the refractory gate. Call between independent clips.
  - `(*Detector).Close() error`.
- **WAV decoding:** `audio.DecodeWAV(b []byte) (pcm []byte, sampleRate, channels int, ok bool)` in `internal/audio/wav.go`. Returns raw S16LE samples. We require 16 kHz mono.
- **ORT model introspection:** `ort.GetInputOutputInfo(path string) ([]ort.InputOutputInfo, []ort.InputOutputInfo, error)`. Each `InputOutputInfo` has `Dimensions ort.Shape` (`[]int64`, a `-1` entry means dynamic) and `DataType ort.TensorElementDataType` (compare against `ort.TensorElementDataTypeFloat`). **It requires the ORT env to be initialized first** (the detector calls it from `newSession`, after `initEnv`).
- **Module path:** `github.com/carroarmato0/nextui-bmo`.
- **Why tests are env-gated:** the bundled ORT `.so` is **linux/aarch64** (the device). On the x86_64 dev box it cannot be `dlopen`ed, so any test that loads ORT is gated behind env vars and **skips** locally. Pure-logic tests run everywhere. This mirrors the existing `internal/wakeword/detector_test.go`. The env vars are: `ONNXRUNTIME_LIB`, `WAKEWORD_MEL`, `WAKEWORD_EMB`, `WAKEWORD_WAKE`, `WAKEWORD_POSITIVE`.

### Build & verification commands (this project)

- Build: `CGO_ENABLED=1 go build ./...` (these packages import cgo via `onnxruntime_go`; `CGO_ENABLED=0` will not compile them).
- Test a package: `CGO_ENABLED=1 go test ./internal/wakeword/ ./cmd/wakeword-eval/`
- Lint (run before every commit): `golangci-lint run ./...`
- Fetch wake-word assets locally (gives you `third_party/wakeword/libonnxruntime.so` + `models/{melspectrogram,embedding_model,hey_bmo}.onnx`): `./scripts/fetch-wakeword-assets.sh`

---

## File Structure

| File | Responsibility |
| --- | --- |
| `internal/wakeword/contract.go` (create) | Exported `InitEnv` + `ValidateClassifier` — the reusable classifier-contract check (also consumed later by spec A). |
| `internal/wakeword/contract_test.go` (create) | Pure `matchShape` table test + env-gated accept/reject tests. |
| `cmd/wakeword-eval/eval.go` (create) | Core evaluation: options/report types, clip loading, scoring, threshold suggestion. No CLI concerns. |
| `cmd/wakeword-eval/main.go` (create) | Flag parsing, calls `Run`, prints the report, sets exit code. |
| `cmd/wakeword-eval/eval_test.go` (create) | Pure tests (helpers, WAV loading) + env-gated integration test. |
| `training/wakeword/config.yaml` (create) | The phrase **override** values applied to openWakeWord's own config. |
| `training/wakeword/README.md` (create) | Human guide: points to openWakeWord's official notebook pinned to a commit, the model contract, mod-author recipe. |

> **Revised (commit `bdd8b52`):** Tasks 6–9 originally shipped a hand-rolled `requirements.txt` + `hey-bmo-training.ipynb`. That notebook targeted an openWakeWord API absent from the pinned 0.5.1 (the config-driven `train.py` exists only in newer versions), used a fabricated piper commit, and assumed piper's old layout. Per the user's decision, we now **point to openWakeWord's official `automatic_model_training.ipynb` pinned to commit `368c03716d1e`** and keep `config.yaml` as override values only; the notebook and `requirements.txt` were removed.
| `assets/wakeword/hey_bmo.onnx` (create, **after training**) | The committed trained classifier. |
| `scripts/fetch-wakeword-assets.sh` (modify) | Stop downloading the `hey_jarvis` placeholder; fetch only base models + `.so`. |
| `scripts/release.sh` (verify/modify) | Confirm the committed classifier is bundled into each pak. |
| `README.md` (modify) | Drop "placeholder" wording; point to the training guide + contract. |

**Phasing.** Tasks 1–9 build tooling and docs and are fully automatable now; they leave the placeholder fetch untouched so builds keep working. **Task 10 is a manual GPU/Colab milestone** that produces the trained model. Tasks 11–15 ship it and must only run once Task 10 has produced a validated `hey_bmo.onnx`.

---

## Task 1: Classifier contract validator in `internal/wakeword`

**Files:**
- Create: `internal/wakeword/contract.go`
- Test: `internal/wakeword/contract_test.go`

- [ ] **Step 1: Write the failing pure-logic test for `matchShape`**

Create `internal/wakeword/contract_test.go`:

```go
package wakeword

import (
	"testing"

	ort "github.com/yalue/onnxruntime_go"
)

func TestMatchShape(t *testing.T) {
	cases := []struct {
		name string
		got  ort.Shape
		want []int64
		ok   bool
	}{
		{"exact", ort.Shape{1, 16, 96}, []int64{1, 16, 96}, true},
		{"dynamic batch matches wildcard", ort.Shape{-1, 16, 96}, []int64{-1, 16, 96}, true},
		{"fixed batch matches wildcard", ort.Shape{1, 16, 96}, []int64{-1, 16, 96}, true},
		{"wrong inner dim", ort.Shape{1, 8, 96}, []int64{-1, 16, 96}, false},
		{"wrong rank", ort.Shape{1, 16}, []int64{-1, 16, 96}, false},
		{"output ok", ort.Shape{1, 1}, []int64{-1, 1}, true},
		{"output wrong", ort.Shape{1, 2}, []int64{-1, 1}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := matchShape(c.got, c.want); got != c.ok {
				t.Fatalf("matchShape(%v,%v)=%v want %v", c.got, c.want, got, c.ok)
			}
		})
	}
}
```

- [ ] **Step 2: Run it to verify it fails (compile error — `matchShape` undefined)**

Run: `CGO_ENABLED=1 go test ./internal/wakeword/ -run TestMatchShape`
Expected: build fails with `undefined: matchShape`.

- [ ] **Step 3: Implement `contract.go`**

Create `internal/wakeword/contract.go`:

```go
package wakeword

import (
	"fmt"

	ort "github.com/yalue/onnxruntime_go"
)

// Wake-word classifier I/O contract: consume 16 openWakeWord embeddings of
// width 96, emit one sigmoid score. A -1 in the want shape is a wildcard so a
// model may declare a fixed (1) or dynamic (-1) batch dimension.
var (
	classifierInputShape  = []int64{-1, classWindow, embDim} // [_,16,96]
	classifierOutputShape = []int64{-1, 1}                   // [_,1]
)

// InitEnv initializes the process-global ONNX Runtime environment from the
// shared library at libPath. Safe to call repeatedly; only the first call has
// effect. New also calls this, so code that only uses New need not call it.
func InitEnv(libPath string) error {
	return initEnv(libPath)
}

// ValidateClassifier reports whether the ONNX model at path matches the
// wake-word classifier contract: a single float32 input [_,16,96] and a single
// float32 output [_,1]. InitEnv (or New) must have run first.
func ValidateClassifier(path string) error {
	ins, outs, err := ort.GetInputOutputInfo(path)
	if err != nil {
		return fmt.Errorf("model info %s: %w", path, err)
	}
	if len(ins) != 1 {
		return fmt.Errorf("classifier %s: want 1 input, got %d", path, len(ins))
	}
	if len(outs) != 1 {
		return fmt.Errorf("classifier %s: want 1 output, got %d", path, len(outs))
	}
	if !matchShape(ins[0].Dimensions, classifierInputShape) {
		return fmt.Errorf("classifier %s: input shape %v, want [_,%d,%d]", path, ins[0].Dimensions, classWindow, embDim)
	}
	if !matchShape(outs[0].Dimensions, classifierOutputShape) {
		return fmt.Errorf("classifier %s: output shape %v, want [_,1]", path, outs[0].Dimensions)
	}
	if ins[0].DataType != ort.TensorElementDataTypeFloat {
		return fmt.Errorf("classifier %s: input dtype %v, want float32", path, ins[0].DataType)
	}
	if outs[0].DataType != ort.TensorElementDataTypeFloat {
		return fmt.Errorf("classifier %s: output dtype %v, want float32", path, outs[0].DataType)
	}
	return nil
}

// matchShape reports whether got matches want, where want entries of -1 are
// wildcards.
func matchShape(got ort.Shape, want []int64) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if want[i] != -1 && got[i] != want[i] {
			return false
		}
	}
	return true
}
```

- [ ] **Step 4: Run the pure test to verify it passes**

Run: `CGO_ENABLED=1 go test ./internal/wakeword/ -run TestMatchShape`
Expected: PASS.

- [ ] **Step 5: Add the env-gated accept/reject tests**

Append to `internal/wakeword/contract_test.go` (these use `os`, so add `"os"` to the file's import block):

```go
func TestValidateClassifierAcceptsWakeModel(t *testing.T) {
	cfg := testConfig(t) // skips unless ONNXRUNTIME_LIB/WAKEWORD_* are set
	if err := InitEnv(cfg.LibraryPath); err != nil {
		t.Fatalf("InitEnv: %v", err)
	}
	if err := ValidateClassifier(cfg.WakeModel); err != nil {
		t.Fatalf("wake model should satisfy the contract: %v", err)
	}
}

func TestValidateClassifierRejectsWrongShape(t *testing.T) {
	cfg := testConfig(t)
	if err := InitEnv(cfg.LibraryPath); err != nil {
		t.Fatalf("InitEnv: %v", err)
	}
	// The melspectrogram model has a different I/O shape than a classifier.
	if err := ValidateClassifier(cfg.MelModel); err == nil {
		t.Fatal("mel model should be rejected by the classifier contract")
	}
}
```

> Note: `testConfig` already lives in `detector_test.go` (same package) and returns a `Config` populated from the env vars, calling `t.Skip` when they are unset. Do not redefine it. The trailing `var _ = os.Getenv` line is only to keep the `os` import used; remove it if `os` is already referenced elsewhere in this file.

- [ ] **Step 6: Verify the env-gated tests (skip locally) and run them for real if possible**

Run (dev box, no aarch64 ORT): `CGO_ENABLED=1 go test ./internal/wakeword/ -run TestValidateClassifier -v`
Expected: both tests `--- SKIP` (env unset / cannot load aarch64 `.so`).

To run for real on the device or an x86_64 ORT host: fetch assets (`./scripts/fetch-wakeword-assets.sh`), then
`ONNXRUNTIME_LIB=third_party/wakeword/libonnxruntime.so WAKEWORD_MEL=third_party/wakeword/models/melspectrogram.onnx WAKEWORD_EMB=third_party/wakeword/models/embedding_model.onnx WAKEWORD_WAKE=third_party/wakeword/models/hey_bmo.onnx CGO_ENABLED=1 go test ./internal/wakeword/ -run TestValidateClassifier -v`
Expected (on a host whose ORT `.so` matches its CPU arch): both PASS.

- [ ] **Step 7: Build, lint, and run the package's existing tests**

Run: `CGO_ENABLED=1 go build ./internal/wakeword/ && CGO_ENABLED=1 go test ./internal/wakeword/ && golangci-lint run ./internal/wakeword/...`
Expected: build OK, tests PASS (env-gated ones skip), lint clean.

- [ ] **Step 8: Commit**

```bash
git add internal/wakeword/contract.go internal/wakeword/contract_test.go
git commit -m "feat(wakeword): export classifier contract validator"
```

---

## Task 2: `wakeword-eval` pure helpers (report math)

**Files:**
- Create: `cmd/wakeword-eval/eval.go`
- Test: `cmd/wakeword-eval/eval_test.go`

- [ ] **Step 1: Write the failing tests for the math helpers**

Create `cmd/wakeword-eval/eval_test.go`:

```go
package main

import (
	"math"
	"testing"
)

func TestFalseAcceptsPerHour(t *testing.T) {
	got := falseAcceptsPerHour(2, 1800) // 2 in half an hour -> 4/hr
	if math.Abs(got-4) > 1e-9 {
		t.Fatalf("got %v want 4", got)
	}
	if falseAcceptsPerHour(5, 0) != 0 {
		t.Fatal("zero duration should yield 0")
	}
}

func TestSuggestThresholdSeparable(t *testing.T) {
	th, ok := suggestThreshold([]float64{0.8, 0.9}, []float64{0.2, 0.3})
	if !ok {
		t.Fatal("expected separable")
	}
	if math.Abs(th-0.55) > 1e-9 { // midpoint of lowestPos(0.8) and highestNeg(0.3)
		t.Fatalf("got %v want 0.55", th)
	}
}

func TestSuggestThresholdOverlap(t *testing.T) {
	if _, ok := suggestThreshold([]float64{0.4}, []float64{0.6}); ok {
		t.Fatal("expected not separable")
	}
	if _, ok := suggestThreshold(nil, []float64{0.1}); ok {
		t.Fatal("no positives -> not separable")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `CGO_ENABLED=1 go test ./cmd/wakeword-eval/`
Expected: build fails — `undefined: falseAcceptsPerHour`, `undefined: suggestThreshold`.

- [ ] **Step 3: Implement the helpers and the option/report types**

Create `cmd/wakeword-eval/eval.go`:

```go
package main

// Options configures an evaluation run.
type Options struct {
	LibraryPath string
	MelModel    string
	EmbModel    string
	Model       string // candidate classifier .onnx
	Positives   string // dir of 16k mono WAVs that SHOULD wake
	Negatives   string // dir of 16k mono WAVs that should NOT wake
	Threshold   float64
	Threads     int
}

// Report holds evaluation results.
type Report struct {
	Positives        int
	PositiveAccepts  int
	Negatives        int
	NegativeSeconds  float64
	FalseAccepts     int
	TrueAcceptRate   float64 // 0..1
	FalseAcceptsHour float64
	SuggestedThresh  float64
	Separable        bool
}

// falseAcceptsPerHour scales a false-accept count over the negative audio
// duration to an hourly rate. Zero duration yields 0.
func falseAcceptsPerHour(falseAccepts int, negativeSeconds float64) float64 {
	if negativeSeconds <= 0 {
		return 0
	}
	return float64(falseAccepts) / negativeSeconds * 3600
}

// suggestThreshold proposes a decision threshold from per-clip max scores. If
// the lowest positive score exceeds the highest negative score the classes are
// separable and the midpoint is returned; otherwise ok is false.
func suggestThreshold(posMax, negMax []float64) (threshold float64, ok bool) {
	if len(posMax) == 0 {
		return 0, false
	}
	lowPos := minSlice(posMax)
	highNeg := 0.0
	if len(negMax) > 0 {
		highNeg = maxSlice(negMax)
	}
	if lowPos > highNeg {
		return (lowPos + highNeg) / 2, true
	}
	return 0, false
}

func minSlice(xs []float64) float64 {
	m := xs[0]
	for _, x := range xs[1:] {
		if x < m {
			m = x
		}
	}
	return m
}

func maxSlice(xs []float64) float64 {
	m := xs[0]
	for _, x := range xs[1:] {
		if x > m {
			m = x
		}
	}
	return m
}
```

- [ ] **Step 4: Run to verify the tests pass**

Run: `CGO_ENABLED=1 go test ./cmd/wakeword-eval/ -run 'TestFalseAcceptsPerHour|TestSuggestThreshold'`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/wakeword-eval/eval.go cmd/wakeword-eval/eval_test.go
git commit -m "feat(wakeword-eval): report math helpers"
```

---

## Task 3: WAV folder loading

**Files:**
- Modify: `cmd/wakeword-eval/eval.go`
- Test: `cmd/wakeword-eval/eval_test.go`

- [ ] **Step 1: Write the failing test for `loadClips` using a hand-built WAV**

Append to `cmd/wakeword-eval/eval_test.go`:

```go
import (
	"encoding/binary"
	"os"
	"path/filepath"
)

// writeWAV writes a minimal 16-bit PCM mono WAV with the given sample rate.
func writeWAV(t *testing.T, path string, sampleRate int, samples []int16) {
	t.Helper()
	var b []byte
	put := func(s string) { b = append(b, s...) }
	u32 := func(v uint32) { var x [4]byte; binary.LittleEndian.PutUint32(x[:], v); b = append(b, x[:]...) }
	u16 := func(v uint16) { var x [2]byte; binary.LittleEndian.PutUint16(x[:], v); b = append(b, x[:]...) }
	dataLen := uint32(len(samples) * 2)
	put("RIFF")
	u32(36 + dataLen)
	put("WAVE")
	put("fmt ")
	u32(16)              // fmt chunk size
	u16(1)               // PCM
	u16(1)               // mono
	u32(uint32(sampleRate))
	u32(uint32(sampleRate * 2)) // byte rate
	u16(2)               // block align
	u16(16)              // bits/sample
	put("data")
	u32(dataLen)
	for _, s := range samples {
		u16(uint16(s))
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("write wav: %v", err)
	}
}

func TestLoadClips(t *testing.T) {
	dir := t.TempDir()
	writeWAV(t, filepath.Join(dir, "b.wav"), 16000, make([]int16, 16000)) // 1.0 s
	writeWAV(t, filepath.Join(dir, "a.wav"), 16000, make([]int16, 8000))  // 0.5 s
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	clips, err := loadClips(dir)
	if err != nil {
		t.Fatalf("loadClips: %v", err)
	}
	if len(clips) != 2 {
		t.Fatalf("want 2 clips, got %d", len(clips))
	}
	if clips[0].name != "a.wav" || clips[1].name != "b.wav" {
		t.Fatalf("clips not sorted by name: %v %v", clips[0].name, clips[1].name)
	}
	if clips[0].seconds < 0.49 || clips[0].seconds > 0.51 {
		t.Fatalf("a.wav seconds %v want ~0.5", clips[0].seconds)
	}
}

func TestLoadClipsRejectsWrongRate(t *testing.T) {
	dir := t.TempDir()
	writeWAV(t, filepath.Join(dir, "bad.wav"), 44100, make([]int16, 4410))
	if _, err := loadClips(dir); err == nil {
		t.Fatal("expected error on non-16k WAV")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `CGO_ENABLED=1 go test ./cmd/wakeword-eval/ -run TestLoadClips`
Expected: build fails — `undefined: loadClips` (and `clip`).

- [ ] **Step 3: Implement `loadClips` + supporting consts/helpers in `eval.go`**

Add to `cmd/wakeword-eval/eval.go` (imports + code):

```go
import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/carroarmato0/nextui-bmo/internal/audio"
)

const (
	hopBytes      = 1280 * 2 // one 80 ms S16LE hop
	scanThreshold = 1e-6     // near-zero so the scan pass reports every hop's score
)

type clip struct {
	name    string
	pcm     []byte // S16LE mono 16k
	seconds float64
}

// loadClips reads every *.wav in dir as 16 kHz mono S16LE, sorted by name.
func loadClips(dir string) ([]clip, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var clips []clip
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".wav" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		pcm, rate, ch, ok := audio.DecodeWAV(raw)
		if !ok {
			return nil, fmt.Errorf("decode wav %s", path)
		}
		if rate != 16000 || ch != 1 {
			return nil, fmt.Errorf("%s: want 16k mono, got rate=%d ch=%d", path, rate, ch)
		}
		clips = append(clips, clip{name: e.Name(), pcm: pcm, seconds: float64(len(pcm)/2) / 16000})
	}
	sort.Slice(clips, func(i, j int) bool { return clips[i].name < clips[j].name })
	return clips, nil
}
```

> The duplicate `import` block: merge these imports into the single import block at the top of `eval.go` rather than adding a second `import (...)`. Likewise merge the test-file imports into one block.

- [ ] **Step 4: Run to verify it passes**

Run: `CGO_ENABLED=1 go test ./cmd/wakeword-eval/ -run TestLoadClips`
Expected: PASS (both subtests).

- [ ] **Step 5: Lint and commit**

Run: `golangci-lint run ./cmd/wakeword-eval/...`
Expected: clean.

```bash
git add cmd/wakeword-eval/eval.go cmd/wakeword-eval/eval_test.go
git commit -m "feat(wakeword-eval): load 16k mono WAV folders"
```

---

## Task 4: Scoring + `Run` (ORT integration, contract-gated)

**Files:**
- Modify: `cmd/wakeword-eval/eval.go`
- Test: `cmd/wakeword-eval/eval_test.go`

- [ ] **Step 1: Implement scoring + `Run` in `eval.go`**

Add to `cmd/wakeword-eval/eval.go` (merge the `wakeword` import into the existing import block):

```go
import "github.com/carroarmato0/nextui-bmo/internal/wakeword"

// chunkPCM splits b into n-byte frames (last frame may be shorter).
func chunkPCM(b []byte, n int) [][]byte {
	var out [][]byte
	for len(b) >= n {
		out = append(out, b[:n])
		b = b[n:]
	}
	if len(b) > 0 {
		out = append(out, b)
	}
	return out
}

// pad prepends/appends silence so short clips have enough context to fill the
// classifier window (matches the detector's own test harness).
func pad(pcm []byte) []byte {
	lead := make([]byte, int(1.0*16000)*2) // 1.0 s
	tail := make([]byte, int(0.5*16000)*2) // 0.5 s
	out := make([]byte, 0, len(lead)+len(pcm)+len(tail))
	out = append(out, lead...)
	out = append(out, pcm...)
	out = append(out, tail...)
	return out
}

// clipFires reports whether the detector fires at least once over pcm.
func clipFires(d *wakeword.Detector, pcm []byte) bool {
	d.Reset()
	fired := false
	for _, frame := range chunkPCM(pcm, hopBytes) {
		if len(d.Write(frame)) > 0 {
			fired = true
		}
	}
	return fired
}

// countFires totals detector firings over pcm (refractory-suppressed, like
// on-device), used to count false accepts.
func countFires(d *wakeword.Detector, pcm []byte) int {
	d.Reset()
	n := 0
	for _, frame := range chunkPCM(pcm, hopBytes) {
		n += len(d.Write(frame))
	}
	return n
}

// maxScores returns each clip's peak hop score using a low-threshold,
// refractory-1 detector so every hop reports.
func maxScores(d *wakeword.Detector, clips []clip) []float64 {
	out := make([]float64, 0, len(clips))
	for _, c := range clips {
		d.Reset()
		m := 0.0
		for _, frame := range chunkPCM(pad(c.pcm), hopBytes) {
			for _, det := range d.Write(frame) {
				if det.Score > m {
					m = det.Score
				}
			}
		}
		out = append(out, m)
	}
	return out
}

// Run evaluates Options.Model against the positive/negative folders. It first
// asserts the model satisfies the classifier contract.
func Run(o Options) (Report, error) {
	if err := wakeword.InitEnv(o.LibraryPath); err != nil {
		return Report{}, err
	}
	if err := wakeword.ValidateClassifier(o.Model); err != nil {
		return Report{}, fmt.Errorf("contract check: %w", err)
	}
	pos, err := loadClips(o.Positives)
	if err != nil {
		return Report{}, fmt.Errorf("positives: %w", err)
	}
	neg, err := loadClips(o.Negatives)
	if err != nil {
		return Report{}, fmt.Errorf("negatives: %w", err)
	}

	base := wakeword.Config{LibraryPath: o.LibraryPath, MelModel: o.MelModel, EmbModel: o.EmbModel, WakeModel: o.Model, Threads: o.Threads}

	atCfg := base
	atCfg.Threshold = o.Threshold
	det, err := wakeword.New(atCfg)
	if err != nil {
		return Report{}, err
	}
	defer det.Close()

	rep := Report{Positives: len(pos), Negatives: len(neg)}
	for _, c := range pos {
		if clipFires(det, pad(c.pcm)) {
			rep.PositiveAccepts++
		}
	}
	for _, c := range neg {
		rep.FalseAccepts += countFires(det, c.pcm)
		rep.NegativeSeconds += c.seconds
	}
	if rep.Positives > 0 {
		rep.TrueAcceptRate = float64(rep.PositiveAccepts) / float64(rep.Positives)
	}
	rep.FalseAcceptsHour = falseAcceptsPerHour(rep.FalseAccepts, rep.NegativeSeconds)

	scanCfg := base
	scanCfg.Threshold = scanThreshold
	scanCfg.RefractorySteps = 1
	scan, err := wakeword.New(scanCfg)
	if err != nil {
		return Report{}, err
	}
	defer scan.Close()
	rep.SuggestedThresh, rep.Separable = suggestThreshold(maxScores(scan, pos), maxScores(scan, neg))
	return rep, nil
}
```

- [ ] **Step 2: Write the env-gated integration test (positive fires, silence doesn't, wrong-shape model rejected)**

Append to `cmd/wakeword-eval/eval_test.go`:

```go
// evalEnv returns ORT lib + base model paths from the environment, skipping if
// unset. WAKEWORD_WAKE is the candidate classifier; WAKEWORD_POSITIVE a clip
// that should fire it.
func evalEnv(t *testing.T) Options {
	t.Helper()
	o := Options{
		LibraryPath: os.Getenv("ONNXRUNTIME_LIB"),
		MelModel:    os.Getenv("WAKEWORD_MEL"),
		EmbModel:    os.Getenv("WAKEWORD_EMB"),
		Model:       os.Getenv("WAKEWORD_WAKE"),
		Threshold:   0.5,
		Threads:     2,
	}
	if o.LibraryPath == "" || o.MelModel == "" || o.EmbModel == "" || o.Model == "" {
		t.Skip("set ONNXRUNTIME_LIB, WAKEWORD_MEL, WAKEWORD_EMB, WAKEWORD_WAKE to run")
	}
	return o
}

func TestRunPositiveAndSilence(t *testing.T) {
	o := evalEnv(t)
	posClip := os.Getenv("WAKEWORD_POSITIVE")
	if posClip == "" {
		t.Skip("set WAKEWORD_POSITIVE to a clip that fires WAKEWORD_WAKE")
	}
	// Build positive/negative folders: the positive clip, and 5 s of silence.
	posDir := t.TempDir()
	negDir := t.TempDir()
	raw, err := os.ReadFile(posClip)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(posDir, "pos.wav"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	writeWAV(t, filepath.Join(negDir, "silence.wav"), 16000, make([]int16, 16000*5))

	o.Positives = posDir
	o.Negatives = negDir
	rep, err := Run(o)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.PositiveAccepts != 1 {
		t.Fatalf("expected the positive clip to fire, got %d/%d", rep.PositiveAccepts, rep.Positives)
	}
	if rep.FalseAccepts != 0 {
		t.Fatalf("silence produced %d false accepts", rep.FalseAccepts)
	}
	t.Logf("true-accept=%.0f%% false/hr=%.2f suggested=%.3f separable=%v",
		rep.TrueAcceptRate*100, rep.FalseAcceptsHour, rep.SuggestedThresh, rep.Separable)
}

func TestRunRejectsWrongShapeModel(t *testing.T) {
	o := evalEnv(t)
	o.Model = o.MelModel // wrong I/O shape for a classifier
	o.Positives = t.TempDir()
	o.Negatives = t.TempDir()
	if _, err := Run(o); err == nil {
		t.Fatal("expected Run to reject a model that violates the contract")
	}
}
```

- [ ] **Step 3: Verify (skips locally; run for real on-device/x86_64 ORT)**

Run: `CGO_ENABLED=1 go test ./cmd/wakeword-eval/ -run TestRun -v`
Expected (dev box): `--- SKIP`.

For real (matching-arch ORT host), after `./scripts/fetch-wakeword-assets.sh` and recording a "hey jarvis" clip to `/tmp/pos.wav` (16k mono):
```bash
ONNXRUNTIME_LIB=third_party/wakeword/libonnxruntime.so \
WAKEWORD_MEL=third_party/wakeword/models/melspectrogram.onnx \
WAKEWORD_EMB=third_party/wakeword/models/embedding_model.onnx \
WAKEWORD_WAKE=third_party/wakeword/models/hey_bmo.onnx \
WAKEWORD_POSITIVE=/tmp/pos.wav \
CGO_ENABLED=1 go test ./cmd/wakeword-eval/ -run TestRun -v
```
Expected: `TestRunPositiveAndSilence` PASS, `TestRunRejectsWrongShapeModel` PASS.

- [ ] **Step 4: Build, lint**

Run: `CGO_ENABLED=1 go build ./cmd/wakeword-eval/ && golangci-lint run ./cmd/wakeword-eval/...`
Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add cmd/wakeword-eval/eval.go cmd/wakeword-eval/eval_test.go
git commit -m "feat(wakeword-eval): score clips and gate on the model contract"
```

---

## Task 5: `wakeword-eval` CLI (`main.go`)

**Files:**
- Create: `cmd/wakeword-eval/main.go`

- [ ] **Step 1: Implement the CLI**

Create `cmd/wakeword-eval/main.go`:

```go
// Command wakeword-eval scores a candidate wake-word classifier (.onnx)
// against folders of 16 kHz mono WAVs using the on-device detector and the
// bundled openWakeWord base models. It reports true-accept rate, false
// accepts (and an hourly estimate), and a suggested threshold, and exits
// non-zero if the model violates the [1,16,96]->[1,1] contract or evaluation
// finds no clean separating threshold.
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	var o Options
	flag.StringVar(&o.LibraryPath, "lib", "third_party/wakeword/libonnxruntime.so", "path to libonnxruntime.so")
	flag.StringVar(&o.MelModel, "mel", "third_party/wakeword/models/melspectrogram.onnx", "path to melspectrogram.onnx")
	flag.StringVar(&o.EmbModel, "emb", "third_party/wakeword/models/embedding_model.onnx", "path to embedding_model.onnx")
	flag.StringVar(&o.Model, "model", "", "path to the candidate classifier .onnx (required)")
	flag.StringVar(&o.Positives, "positives", "", "dir of WAVs that SHOULD wake (required)")
	flag.StringVar(&o.Negatives, "negatives", "", "dir of WAVs that should NOT wake (required)")
	flag.Float64Var(&o.Threshold, "threshold", 0.5, "decision threshold for accept/false-accept counts")
	flag.IntVar(&o.Threads, "threads", 2, "ONNX Runtime intra-op threads")
	flag.Parse()

	if o.Model == "" || o.Positives == "" || o.Negatives == "" {
		fmt.Fprintln(os.Stderr, "wakeword-eval: -model, -positives and -negatives are required")
		flag.Usage()
		os.Exit(2)
	}

	rep, err := Run(o)
	if err != nil {
		fmt.Fprintf(os.Stderr, "wakeword-eval: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("model:            %s\n", o.Model)
	fmt.Printf("threshold:        %.3f\n", o.Threshold)
	fmt.Printf("positives:        %d  accepted %d  (%.1f%% true-accept)\n", rep.Positives, rep.PositiveAccepts, rep.TrueAcceptRate*100)
	fmt.Printf("negatives:        %d clips, %.1f s\n", rep.Negatives, rep.NegativeSeconds)
	fmt.Printf("false accepts:    %d  (~%.2f / hour)\n", rep.FalseAccepts, rep.FalseAcceptsHour)
	if rep.Separable {
		fmt.Printf("suggested thresh: %.3f\n", rep.SuggestedThresh)
	} else {
		fmt.Printf("suggested thresh: (none — positive/negative scores overlap)\n")
	}

	// Non-zero exit when the classes don't separate, so CI / scripts can gate.
	if !rep.Separable {
		os.Exit(3)
	}
}
```

- [ ] **Step 2: Verify it builds and shows usage**

Run: `CGO_ENABLED=1 go build ./cmd/wakeword-eval/ && CGO_ENABLED=1 go run ./cmd/wakeword-eval -h`
Expected: build OK; usage lists `-lib -mel -emb -model -positives -negatives -threshold -threads`.

- [ ] **Step 3: Verify missing-arg handling**

Run: `CGO_ENABLED=1 go run ./cmd/wakeword-eval; echo "exit=$?"`
Expected: prints the "required" message + usage, `exit=2`.

- [ ] **Step 4: Lint and commit**

Run: `golangci-lint run ./cmd/wakeword-eval/...`
Expected: clean.

```bash
git add cmd/wakeword-eval/main.go
git commit -m "feat(wakeword-eval): CLI entrypoint"
```

---

## Task 6: `training/wakeword/config.yaml`

**Files:**
- Create: `training/wakeword/config.yaml`

- [ ] **Step 1: Write the config**

Create `training/wakeword/config.yaml`:

```yaml
# Hey BMO wake-word training config (openWakeWord v0.5.1 training-config schema).
#
# This is the ONLY file a follower edits to retarget a different wake phrase:
# change `target_phrase` (and `model_name` / `output_dir`) and re-run the
# notebook. The notebook reads these values; nothing about the phrase is
# hard-coded elsewhere.

# --- Phrase ---------------------------------------------------------------
# Spell the phrase the way Piper should *pronounce* it. "Beemo" reads more
# reliably than "BMO". Audition the first batch (notebook step 1) and adjust
# these spellings before the full synthesis run.
target_phrase:
  - "hey beemo"
  - "hey bee-moh"
model_name: "hey_bmo"

# --- Dataset sizes --------------------------------------------------------
n_samples: 30000          # synthetic positives
n_samples_val: 2000       # held-out synthetic positives
tts_batch_size: 50

# --- Augmentation ---------------------------------------------------------
augmentation_batch_size: 16
augmentation_rounds: 1
# Room impulse responses + background noise (downloaded by the notebook).
rir_paths:
  - "./mit_rirs"
background_paths:
  - "./audioset_16k"
  - "./fma"

# --- Negative / false-positive data (downloaded by the notebook) ---------
false_positive_validation_data_path: "./validation_set_features.npy"
feature_data_files:
  ACAV100M_sample: "./openwakeword_features_ACAV100M_2000_hrs_16bit.npy"

# --- Model ----------------------------------------------------------------
model_type: "dnn"
layer_size: 32
steps: 50000
max_negative_weight: 1500
target_accuracy: 0.7
target_recall: 0.5

# --- Output ---------------------------------------------------------------
# openWakeWord writes <model_name>.onnx (and .tflite) into output_dir. We ship
# the .onnx; copy it to assets/wakeword/hey_bmo.onnx after validating.
output_dir: "./hey_bmo_out"
```

- [ ] **Step 2: Commit**

```bash
git add training/wakeword/config.yaml
git commit -m "feat(training): hey-bmo wake-word training config"
```

---

## Task 7: `training/wakeword/requirements.txt`

**Files:**
- Create: `training/wakeword/requirements.txt`

- [ ] **Step 1: Write pinned requirements**

Create `training/wakeword/requirements.txt`:

```text
# Pinned for reproducible "Hey BMO" training. openWakeWord 0.5.1 is the version
# whose base melspectrogram/embedding models the pak bundles — keep this in sync
# with OWW_VERSION in scripts/fetch-wakeword-assets.sh.
openwakeword==0.5.1
torch==2.2.2
onnx==1.16.0
onnxruntime==1.26.0
audiomentations==0.35.0
numpy==1.26.4
pyyaml==6.0.1
tqdm==4.66.4
scipy==1.13.1
# piper-sample-generator is installed from GitHub in the notebook (no PyPI
# release); the notebook pins it to a specific commit.
```

- [ ] **Step 2: Commit**

```bash
git add training/wakeword/requirements.txt
git commit -m "feat(training): pin wake-word training dependencies"
```

---

## Task 8: `training/wakeword/README.md`

**Files:**
- Create: `training/wakeword/README.md`

- [ ] **Step 1: Write the guide**

Create `training/wakeword/README.md`:

````markdown
# Training a "Hey BMO" wake word

This directory holds a pinned, reproducible pipeline that produces BMO's
on-device wake-word classifier. Run it on **Google Colab (free GPU)** or a
**local NVIDIA GPU**. Anyone — including mod authors who want a different wake
phrase — can run it by editing one file (`config.yaml`).

## What you get

A single `hey_bmo.onnx` classifier (~1.3 MB) that plugs into BMO's existing
detector. The detector runs three models in series — a shared **melspectrogram**
model, a shared **embedding** model, and this **classifier**. Only the
classifier is phrase-specific; you do not retrain the base models.

## Model contract (read this if you're a mod author)

A classifier is compatible with BMO iff:

- it is an ONNX model with input `[1, 16, 96]` float32 (16 openWakeWord
  embeddings ≈ 1.28 s of audio) and output `[1, 1]` float32 (a sigmoid score), and
- it was trained against openWakeWord's **v0.5.1** base melspectrogram +
  embedding models — the exact ones the pak bundles in `assets/wakeword/`.

The repo tool `cmd/wakeword-eval` asserts this contract and is the gate before
shipping any model.

## Prerequisites

- A GPU. Colab's free tier is enough (expect ~1–2 h end to end). A local NVIDIA
  GPU works too.
- The pinned deps in `requirements.txt`.
- Disk/time for the negative-feature download (several GB; cached after the
  first run).

## Steps

1. Open `hey-bmo-training.ipynb` in Colab (Runtime → GPU) or Jupyter on your GPU box.
2. Run the **setup** cells: they install `requirements.txt`, clone
   piper-sample-generator at the pinned commit, and download the openWakeWord
   negative features, RIRs, and noise sets.
3. **Audition** (first synthesis cell): listen to ~10 generated "Hey Beemo"
   clips. If the pronunciation is wrong, edit `target_phrase` in `config.yaml`
   (e.g. `"hey bee-moh"`, `"hey BEE moh"`) and re-run the cell.
4. Run the **synthesize → augment → train** cells. They read `config.yaml`.
5. The notebook exports `hey_bmo.onnx` and prints held-out accuracy,
   false-accepts/hour, and a recommended threshold.
6. Download `hey_bmo.onnx`.

## Validate before shipping

From the repo root, with the base assets fetched (`./scripts/fetch-wakeword-assets.sh`):

```bash
CGO_ENABLED=1 go run ./cmd/wakeword-eval \
  -model /path/to/hey_bmo.onnx \
  -positives /path/to/positive_wavs \
  -negatives /path/to/negative_wavs \
  -threshold 0.5
```

`-positives` is a folder of 16 kHz mono WAVs of people saying "Hey BMO";
`-negatives` is unrelated speech/noise. The tool prints true-accept %,
false-accepts/hour, and a suggested threshold, and exits non-zero if the model
breaks the contract or the classes don't separate.

Then do an on-device check: copy the candidate to the deployed pak's
`assets/wakeword/hey_bmo.onnx`, enable **Settings → WAKE WORD**, say
"Hey Beemo", and watch `scripts/debug-logs.sh` for `wake word detected:
score=…`. Adjust the threshold if needed.

## Retargeting (mod authors)

Copy this directory, change `target_phrase` (and `model_name`) in `config.yaml`,
and run the notebook. The output ONNX obeys the same contract above, so it loads
into BMO unchanged.
````

- [ ] **Step 2: Commit**

```bash
git add training/wakeword/README.md
git commit -m "docs(training): hey-bmo training guide and model contract"
```

---

## Task 9: `training/wakeword/hey-bmo-training.ipynb`

**Files:**
- Create: `training/wakeword/hey-bmo-training.ipynb`

> **Integration note for the executor:** this notebook drives the *external*
> openWakeWord v0.5.1 automatic-training interface and piper-sample-generator.
> Those tools are not runnable on this dev box (no GPU; the bundled ORT is
> aarch64). The notebook is a faithful, pinned scaffold; its external calls are
> confirmed/adjusted on the **first Colab run** (Task 10), which is exactly what
> the "audition" step surfaces. Do not invent results — the notebook only
> *prints* metrics produced by the tools.

- [ ] **Step 1: Write the notebook**

Create `training/wakeword/hey-bmo-training.ipynb` with this content:

```json
{
 "cells": [
  {
   "cell_type": "markdown",
   "metadata": {},
   "source": [
    "# Hey BMO wake-word training\n",
    "Pinned openWakeWord v0.5.1 pipeline. Run top to bottom on a GPU runtime.\n",
    "Edit only `config.yaml` to change the phrase. See README.md for the model contract."
   ]
  },
  {
   "cell_type": "markdown",
   "metadata": {},
   "source": ["## 1. Setup: install pinned deps + tools"]
  },
  {
   "cell_type": "code",
   "execution_count": null,
   "metadata": {},
   "outputs": [],
   "source": [
    "import sys, subprocess, os\n",
    "subprocess.run([sys.executable, '-m', 'pip', 'install', '-q', '-r', 'requirements.txt'], check=True)\n",
    "# piper-sample-generator has no PyPI release; pin to a known-good commit.\n",
    "PIPER_COMMIT = 'b8d72e5'\n",
    "if not os.path.isdir('piper-sample-generator'):\n",
    "    subprocess.run(['git', 'clone', 'https://github.com/rhasspy/piper-sample-generator'], check=True)\n",
    "    subprocess.run(['git', '-C', 'piper-sample-generator', 'checkout', PIPER_COMMIT], check=True)\n",
    "print('deps installed')"
   ]
  },
  {
   "cell_type": "code",
   "execution_count": null,
   "metadata": {},
   "outputs": [],
   "source": [
    "import yaml\n",
    "with open('config.yaml') as f:\n",
    "    cfg = yaml.safe_load(f)\n",
    "print('phrase:', cfg['target_phrase'])\n",
    "print('model :', cfg['model_name'])\n",
    "print('samples:', cfg['n_samples'], 'val:', cfg['n_samples_val'])"
   ]
  },
  {
   "cell_type": "markdown",
   "metadata": {},
   "source": ["## 2. Download openWakeWord training data (negatives, RIRs, noise)"]
  },
  {
   "cell_type": "code",
   "execution_count": null,
   "metadata": {},
   "outputs": [],
   "source": [
    "# openWakeWord ships helpers to fetch its precomputed negative features and\n",
    "# augmentation sets. This downloads several GB on first run (cached after).\n",
    "import openwakeword\n",
    "from openwakeword import train\n",
    "train.download_training_data() if hasattr(train, 'download_training_data') else print(\n",
    "    'Use the download cells from the pinned openWakeWord automatic_model_training notebook')\n",
    "print('openWakeWord', openwakeword.__version__)"
   ]
  },
  {
   "cell_type": "markdown",
   "metadata": {},
   "source": [
    "## 3. Audition: synthesize a small batch and listen\n",
    "If the pronunciation is wrong, edit `target_phrase` in `config.yaml`, re-run cell 1's config load, then re-run this cell."
   ]
  },
  {
   "cell_type": "code",
   "execution_count": null,
   "metadata": {},
   "outputs": [],
   "source": [
    "import subprocess, glob\n",
    "os.makedirs('audition', exist_ok=True)\n",
    "subprocess.run([sys.executable, 'piper-sample-generator/generate_samples.py',\n",
    "                cfg['target_phrase'][0], '--max-samples', '10',\n",
    "                '--output-dir', 'audition'], check=True)\n",
    "from IPython.display import Audio, display\n",
    "for w in sorted(glob.glob('audition/*.wav'))[:5]:\n",
    "    print(w); display(Audio(w))"
   ]
  },
  {
   "cell_type": "markdown",
   "metadata": {},
   "source": ["## 4. Synthesize → augment → train (reads config.yaml)"]
  },
  {
   "cell_type": "code",
   "execution_count": null,
   "metadata": {},
   "outputs": [],
   "source": [
    "# openWakeWord's automatic training entrypoint. It synthesizes positives via\n",
    "# piper, augments with the RIR/noise sets, computes base mel->embedding\n",
    "# features, trains the classifier, and exports ONNX into cfg['output_dir'].\n",
    "subprocess.run([sys.executable, '-m', 'openwakeword.train',\n",
    "                '--training_config', 'config.yaml',\n",
    "                '--generate_clips', '--augment_clips', '--train_model'], check=True)"
   ]
  },
  {
   "cell_type": "markdown",
   "metadata": {},
   "source": ["## 5. Locate the exported model + print metrics"]
  },
  {
   "cell_type": "code",
   "execution_count": null,
   "metadata": {},
   "outputs": [],
   "source": [
    "import glob\n",
    "onnx = sorted(glob.glob(os.path.join(cfg['output_dir'], '**', cfg['model_name'] + '.onnx'), recursive=True))\n",
    "assert onnx, 'no exported ONNX found; check the training cell output'\n",
    "print('exported:', onnx[-1])\n",
    "print('Download this file and validate with cmd/wakeword-eval (see README.md),')\n",
    "print('then copy it to assets/wakeword/hey_bmo.onnx in the repo.')"
   ]
  }
 ],
 "metadata": {
  "kernelspec": {"display_name": "Python 3", "language": "python", "name": "python3"},
  "language_info": {"name": "python", "version": "3.10"}
 },
 "nbformat": 4,
 "nbformat_minor": 5
}
```

- [ ] **Step 2: Validate the notebook is well-formed JSON**

Run: `python3 -c "import json,sys; json.load(open('training/wakeword/hey-bmo-training.ipynb')); print('ok')"`
Expected: `ok`.

- [ ] **Step 3: Commit**

```bash
git add training/wakeword/hey-bmo-training.ipynb
git commit -m "feat(training): pinned hey-bmo training notebook"
```

---

## Task 10: 🧑‍🔧 MANUAL MILESTONE — train and validate the model

> **This task is performed by a human on a GPU/Colab runtime; it produces no
> repo code.** Tasks 11–15 must not run until this yields a validated
> `hey_bmo.onnx`. An agent executing this plan should **stop here and hand back
> to the user** with these instructions.

- [ ] **Step 1: Run openWakeWord's official notebook** pinned to commit `368c03716d1e` on Colab (GPU): <https://colab.research.google.com/github/dscripka/openWakeWord/blob/368c03716d1e/notebooks/automatic_model_training.ipynb>. Run the Environment setup cell, then in the "Modify values in the config" cell apply BMO's overrides from `training/wakeword/config.yaml` (`target_phrase=["hey beemo"]`, `model_name="hey_bmo"`, `n_samples=20000`, `n_samples_val=2000`, `steps=50000`). Audition the synthetic clips; adjust the spelling if needed. Let it train and export `hey_bmo.onnx`. (See `training/wakeword/README.md`.)

- [ ] **Step 2: Collect evaluation clips.** Record/gather a folder of 16 kHz mono WAVs of "Hey BMO" (positives) and a folder of unrelated speech/noise (negatives). Tip to convert: `ffmpeg -i in.wav -ac 1 -ar 16000 out.wav`.

- [ ] **Step 3: Validate with `wakeword-eval`** (on the device or an x86_64 ORT host — fetch assets first with `./scripts/fetch-wakeword-assets.sh`):

```bash
CGO_ENABLED=1 go run ./cmd/wakeword-eval \
  -model /path/to/hey_bmo.onnx \
  -positives /path/to/positives -negatives /path/to/negatives -threshold 0.5
```
Expected: high true-accept %, ~0 false-accepts/hour, a suggested threshold, exit 0. Note the suggested threshold (used in Task 15).

- [ ] **Step 4: On-device smoke test.** Copy the candidate to the deployed pak's `assets/wakeword/hey_bmo.onnx`, enable **Settings → WAKE WORD**, say "Hey Beemo", and confirm `wake word detected: score=…` in `./scripts/debug-logs.sh`.

- [ ] **Step 5:** Place the validated model at the repo path `assets/wakeword/hey_bmo.onnx` (create the directory) and proceed to Task 11.

---

## Task 11: Commit the trained model

**Files:**
- Create: `assets/wakeword/hey_bmo.onnx` (binary, from Task 10)

> Prereq: Task 10 produced a validated `assets/wakeword/hey_bmo.onnx`. `assets/`
> is tracked and not gitignored (verified: only `third_party/wakeword/` is
> ignored), so the model commits directly.

- [ ] **Step 1: Confirm the file exists and looks like an ONNX model**

Run: `ls -l assets/wakeword/hey_bmo.onnx && head -c 8 assets/wakeword/hey_bmo.onnx | xxd | head -1`
Expected: a file roughly ~1.3 MB; the first bytes are the ONNX/protobuf header (not text).

- [ ] **Step 2: Commit**

```bash
git add assets/wakeword/hey_bmo.onnx
git commit -m "feat(wakeword): ship trained Hey BMO classifier"
```

---

## Task 12: Fetch only the base assets (drop the placeholder)

**Files:**
- Modify: `scripts/fetch-wakeword-assets.sh`

- [ ] **Step 1: Replace the classifier download with base-only**

In `scripts/fetch-wakeword-assets.sh`, change the trailing model-fetch block. Replace:

```bash
# openWakeWord shared pipeline + a stock wake classifier (hey_jarvis) used as
# the "Hey BMO" placeholder until a dedicated model is trained.
fetch "$OWW_BASE/melspectrogram.onnx" "$MODELS/melspectrogram.onnx"
fetch "$OWW_BASE/embedding_model.onnx" "$MODELS/embedding_model.onnx"
fetch "$OWW_BASE/hey_jarvis_v0.1.onnx" "$MODELS/hey_bmo.onnx"
```

with:

```bash
# openWakeWord shared base pipeline only. The "Hey BMO" classifier is NOT
# downloaded — it is the trained model committed at assets/wakeword/hey_bmo.onnx
# and bundled by scripts/release.sh.
fetch "$OWW_BASE/melspectrogram.onnx" "$MODELS/melspectrogram.onnx"
fetch "$OWW_BASE/embedding_model.onnx" "$MODELS/embedding_model.onnx"
```

- [ ] **Step 2: Verify it fetches only the two base models**

Run: `rm -f third_party/wakeword/models/*.onnx; ./scripts/fetch-wakeword-assets.sh && ls third_party/wakeword/models/`
Expected: only `embedding_model.onnx` and `melspectrogram.onnx` (no `hey_bmo.onnx`).

- [ ] **Step 3: Commit**

```bash
git add scripts/fetch-wakeword-assets.sh
git commit -m "build(wakeword): fetch base models only; classifier is committed"
```

---

## Task 13: Verify the release bundles the committed classifier

**Files:**
- Modify (only if the dry check below fails): `scripts/release.sh`

Context: `scripts/release.sh` copies `assets/.` into each pak (line ~144), so the
committed `assets/wakeword/hey_bmo.onnx` lands in `<pak>/assets/wakeword/`, while
the wake-word block (lines ~151–154) copies the fetched base models into the
same dir. Different filenames, no clash. This task verifies that end-to-end.

- [ ] **Step 1: Dry-check the copy logic against a temp dest (no full build)**

Run:
```bash
./scripts/fetch-wakeword-assets.sh
dest=$(mktemp -d)
mkdir -p "$dest/assets" "$dest/lib/tg5040"
cp -R assets/. "$dest/assets/"
cp third_party/wakeword/libonnxruntime.so "$dest/lib/tg5040/libonnxruntime.so"
mkdir -p "$dest/assets/wakeword"; cp third_party/wakeword/models/*.onnx "$dest/assets/wakeword/"
ls "$dest/assets/wakeword/"
```
Expected: `embedding_model.onnx  hey_bmo.onnx  melspectrogram.onnx` (all three present). If `hey_bmo.onnx` is missing, the `cp -R assets/.` step in `release.sh` is not picking it up — fix `release.sh` to explicitly `cp assets/wakeword/hey_bmo.onnx "$dest/assets/wakeword/"` in `copy_pak` and the tg5050 block, then re-run this check.

- [ ] **Step 2: Full release build smoke test**

Run: `./scripts/release.sh > /tmp/bmo-release.log 2>&1; echo "exit=$?"; tail -5 /tmp/bmo-release.log`
Expected: `exit=0`. Then verify the model is in the built pak:
`find dist -path '*assets/wakeword/hey_bmo.onnx'`
Expected: at least one match under `dist/`.

> If `.go_cache/` permission errors appear (Docker writes it as root), run `chmod -R u+w .go_cache` and retry. Do not `rm -rf` it.

- [ ] **Step 3: Commit (only if `release.sh` was modified)**

```bash
git add scripts/release.sh
git commit -m "build(wakeword): bundle committed Hey BMO classifier into the pak"
```

---

## Task 14: Update README wording

**Files:**
- Modify: `README.md` (the "Wake word (hands-free)" subsection, ~lines 155–173)

- [ ] **Step 1: Replace the placeholder bullet + add a training pointer**

In `README.md`, replace this line:

```markdown
- The shipped model is a stock "hey jarvis" placeholder until a dedicated
  "Hey BMO" model is trained.
```

with:

```markdown
- The shipped model is BMO's own **"Hey BMO"** classifier (say "Beemo"). You can
  train your own wake phrase with the documented, GPU/Colab pipeline in
  [`training/wakeword/`](training/wakeword/README.md) — it produces a drop-in
  model that obeys the same `[1,16,96] → [1,1]` contract.
```

- [ ] **Step 2: Verify the link target exists**

Run: `test -f training/wakeword/README.md && echo ok`
Expected: `ok`.

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: README points to the Hey BMO training pipeline"
```

---

## Task 15: (Conditional) Update the detector default threshold

> Only do this if Task 10's `wakeword-eval` recommended a threshold meaningfully
> different from the current default (0.5). Otherwise skip this task.

**Files:**
- Modify: `internal/wakeword/detector.go` (the `defaultThreshold` constant)

- [ ] **Step 1: Update the constant**

In `internal/wakeword/detector.go`, change the `defaultThreshold` constant to the value recommended by `wakeword-eval` (e.g. `0.6`). Keep the comment explaining it is the evaluation-informed default.

- [ ] **Step 2: Build, test, lint**

Run: `CGO_ENABLED=1 go build ./internal/wakeword/ && CGO_ENABLED=1 go test ./internal/wakeword/ && golangci-lint run ./internal/wakeword/...`
Expected: build OK, tests PASS, lint clean.

- [ ] **Step 3: Commit**

```bash
git add internal/wakeword/detector.go
git commit -m "tune(wakeword): set evaluation-informed default threshold"
```

---

## Done

After Task 15 (or Task 14 if no threshold change), all deliverables exist:
`cmd/wakeword-eval`, the `training/wakeword/` pipeline, the committed
`assets/wakeword/hey_bmo.onnx`, the base-only fetch script, and updated docs.
Run the finishing-a-development-branch skill to verify the full suite
(`CGO_ENABLED=1 go test ./...`), then merge `feat/hey-bmo-training` back to main.
```
