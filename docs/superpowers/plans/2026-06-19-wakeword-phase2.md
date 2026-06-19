# On-Device Wake-Word (Phase 2) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an optional, always-on "Hey BMO" wake-word that triggers the existing voice pipeline hands-free, plus a "continued conversation" window so two BMOs (or a person and BMO) can talk without re-triggering — all running on-device via onnxruntime, gated on the now-passed P2.0 spike.

**Architecture:** A new `internal/wakeword` package wraps the openWakeWord ONNX pipeline (melspectrogram → speech-embedding → classifier) behind `onnxruntime_go`, consuming 16 kHz mono S16LE frames. `internal/audio.CaptureRouter` gains a fan-out so the detector and the PTT buffer can both read every capture batch. When the detector fires while BMO is idle, `cmd/bmo-pak` drives the **same** `pipeline.ProcessBatch` path PTT already uses. Detection is suppressed during BMO's own TTS playback (+ a guard tail) so BMO never wakes on itself. A small `internal/power` package requests the `performance` CPU governor during STT/TTS bursts and restores it after. The ONNX runtime `.so` and models ship as pak assets, loaded at runtime via `SetSharedLibraryPath`.

**Tech Stack:** Go, `github.com/yalue/onnxruntime_go` v1.31.0 (CGo, `dlopen`s `libonnxruntime.so`), openWakeWord v0.5.1 ONNX models, existing `internal/audio` / `internal/assistant` / `internal/config` / `internal/ui` packages.

**Spec:** `docs/superpowers/specs/2026-06-19-self-hosted-stt-tts-and-wake-word-design.md` (Phase 2, P2.1–P2.9).
**Feasibility:** `docs/superpowers/2026-06-19-p2.0-wakeword-feasibility-findings.md` (PASS; use **2 intra-op threads**, ship `.so` as asset, RSS ~40 MiB, ~19 ms/frame).

**Conventions (from CLAUDE.md / project memory):**
- Pure-Go packages test with `CGO_ENABLED=0 go test ./internal/<pkg>/ -run <Name> -v`. The wakeword package needs CGo + the ORT `.so`: `CGO_ENABLED=1 go test ./internal/wakeword/ -run <Name> -v` with `ONNXRUNTIME_LIB` / model paths via test env (skip if unset, see Task 4).
- Full suite before finishing: `CGO_ENABLED=1 go test ./...`.
- `golangci-lint run ./...` after every change; new code adds no findings.
- Commit messages: **no** `Co-Authored-By` trailer.
- TrimUI buttons: A=BTN_EAST(305) confirm/PTT, B=BTN_SOUTH(304) cancel.
- Branch: continue on `spike/p2.0-wakeword-feasibility` (carries the spike + findings) or branch fresh from it; do not touch other worktrees.

---

## File Structure

- `internal/audio/router.go` — **modify.** Add `Subscribe() (<-chan []byte, func())` fan-out. The internal `run()` loop broadcasts each batch to all registered subscribers instead of (or in addition to) the single `batches` channel. `Batches()` stays for back-compat as one subscriber.
- `internal/audio/router_test.go` — **modify.** Multi-subscriber delivery test.
- `internal/audio/pcm.go` — **new.** `S16LEToFloat32(pcm []byte) []float32` helper (mono). Lives by `level.go`/`resample.go`.
- `internal/audio/pcm_test.go` — **new.**
- `internal/wakeword/detector.go` — **new.** `Detector` (3 ORT sessions, streaming framing, threshold, debounce). Core of P2.2.
- `internal/wakeword/detector_test.go` — **new.** Synthetic-frame threshold/event tests + real-clip correctness (gated on env).
- `internal/wakeword/doc.go` — **new.** Package doc + asset-path expectations.
- `internal/power/governor.go` — **new.** `Governor` requests/restores the `performance` scaling governor via `/sys`. P2.7.
- `internal/power/governor_test.go` — **new.** Uses a temp fake sysfs root.
- `internal/config/config.go` — **modify.** Add `WakeWordEnabled bool` and `ContinuedConversation string` (enum: off/short/long) fields + `Normalize`/`Validate`/`Default` handling. (`InputTriggerWakeWord` already exists.)
- `internal/config/config_test.go` — **modify.**
- `internal/ui/settings_menu.go` — **modify.** Add WAKE WORD toggle + CONTINUED CONVO cycle rows; bump `settingsSlotCount`; handle in `ToggleFocused`/`Cycle`. P2.8.
- `internal/ui/settings_menu_test.go` — **modify.**
- `cmd/bmo-pak/wakeword.go` — **new.** `startWakeWord(...)` mirroring `startPushToTalk`: subscribe to capture, run detector while idle+enabled, gate during speaking+tail, drive ProcessBatch, manage the continued-conversation window, drive the governor.
- `cmd/bmo-pak/wakeword_test.go` — **new.** Window/gating state-machine tests with a fake detector + clock.
- `cmd/bmo-pak/main.go` — **modify.** Wire `startWakeWord` next to `startPushToTalk`; load detector assets; pass governor to the voice pipeline burst path.
- `scripts/release.sh` — **modify.** Bundle `libonnxruntime.so` (aarch64) into `lib/<platform>/` and the openWakeWord models into `assets/wakeword/`, like `libSDL2`.
- `docs/MODDING.md` / `README.md` — **modify.** Document the wake-word setting and that the model is a stock "hey jarvis" until a "Hey BMO" model is trained (follow-up).

**Asset layout on device** (resolved open question — ship as assets, not embedded):
```
BMO.pak/lib/<platform>/libonnxruntime.so
BMO.pak/assets/wakeword/melspectrogram.onnx
BMO.pak/assets/wakeword/embedding_model.onnx
BMO.pak/assets/wakeword/hey_bmo.onnx   # stock hey_jarvis_v0.1.onnx renamed until trained
```

---

## Task 1: Capture fan-out (`audio.CaptureRouter.Subscribe`) — P2.1

**Files:**
- Modify: `internal/audio/router.go`
- Test: `internal/audio/router_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/audio/router_test.go`:

```go
func TestRouterFanOutDeliversToAllSubscribers(t *testing.T) {
	src := newFakePCMSource() // existing test helper in this file
	r := NewCaptureRouter(src, 4) // tiny batchLimit so each frame flushes
	if err := r.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	subA, cancelA := r.Subscribe()
	subB, cancelB := r.Subscribe()
	defer cancelA()
	defer cancelB()

	src.push([]byte{1, 2, 3, 4, 5, 6, 7, 8})

	for _, sub := range []<-chan []byte{subA, subB} {
		select {
		case b := <-sub:
			if len(b) == 0 {
				t.Fatalf("empty batch")
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber did not receive batch")
		}
	}
}

func TestRouterSubscribeAfterCancelStopsDelivery(t *testing.T) {
	src := newFakePCMSource()
	r := NewCaptureRouter(src, 4)
	_ = r.Start()
	sub, cancel := r.Subscribe()
	cancel()
	src.push([]byte{1, 2, 3, 4})
	select {
	case _, ok := <-sub:
		if ok {
			t.Fatalf("expected closed channel after cancel")
		}
	case <-time.After(500 * time.Millisecond):
		// also acceptable: no delivery
	}
}
```

If `newFakePCMSource`/`push` do not exist, check `router_test.go` for the existing fake source and reuse its real name; adapt the calls. Do not invent a second fake.

- [ ] **Step 2: Run test to verify it fails**

Run: `CGO_ENABLED=0 go test ./internal/audio/ -run TestRouterFanOut -v`
Expected: FAIL — `r.Subscribe undefined`.

- [ ] **Step 3: Implement fan-out**

In `internal/audio/router.go`, replace the single-`batches`-channel broadcast with a subscriber set. Keep `Batches()` working by registering one permanent subscriber lazily.

```go
type subscriber struct {
	ch chan []byte
}

// add to CaptureRouter struct:
//   subs      map[*subscriber]struct{}
//   legacy    *subscriber // backs Batches()
// (keep mu, done, levels, errors, closed; remove the standalone `batches` field)

// In NewCaptureRouter, initialize: subs: make(map[*subscriber]struct{}).

// Subscribe registers a new batch consumer. The returned cancel func
// unregisters and closes the channel. Each subscriber gets every batch;
// a slow subscriber drops batches (non-blocking send) rather than stalling
// capture — matching the existing best-effort Batches() semantics.
func (r *CaptureRouter) Subscribe() (<-chan []byte, func()) {
	if r == nil {
		ch := make(chan []byte)
		close(ch)
		return ch, func() {}
	}
	s := &subscriber{ch: make(chan []byte, 4)}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		close(s.ch)
		return s.ch, func() {}
	}
	r.subs[s] = struct{}{}
	r.mu.Unlock()
	var once sync.Once
	cancel := func() {
		once.Do(func() {
			r.mu.Lock()
			if _, ok := r.subs[s]; ok {
				delete(r.subs, s)
				close(s.ch)
			}
			r.mu.Unlock()
		})
	}
	return s.ch, cancel
}

// Batches returns a channel backed by a lazily-created permanent subscriber.
func (r *CaptureRouter) Batches() <-chan []byte {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.legacy == nil {
		r.legacy = &subscriber{ch: make(chan []byte, 4)}
		if !r.closed {
			r.subs[r.legacy] = struct{}{}
		}
	}
	return r.legacy.ch
}
```

In `run()`, replace the `flush()` body's `select { case r.batches <- batch: default: }` with a broadcast:

```go
flush := func() {
	if len(buffer) == 0 {
		return
	}
	batch := make([]byte, len(buffer))
	copy(batch, buffer)
	buffer = buffer[:0]
	r.mu.RLock()
	for s := range r.subs {
		select {
		case s.ch <- batch:
		default:
		}
	}
	r.mu.RUnlock()
}
```

And in `run()`'s deferred cleanup, replace `defer close(r.batches)` with closing every subscriber:

```go
defer func() {
	r.mu.Lock()
	r.closed = true
	for s := range r.subs {
		close(s.ch)
	}
	r.subs = map[*subscriber]struct{}{}
	r.mu.Unlock()
}()
```

(Keep `defer close(r.done)`, `defer close(r.levels)`, `defer close(r.errors)`.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `CGO_ENABLED=0 go test ./internal/audio/ -run 'TestRouter' -v`
Expected: PASS (new fan-out tests + existing router tests).

- [ ] **Step 5: Lint + commit**

```bash
golangci-lint run ./internal/audio/
git add internal/audio/router.go internal/audio/router_test.go
git commit -m "feat(audio): CaptureRouter.Subscribe fan-out for multiple batch consumers"
```

---

## Task 2: S16LE→float32 helper

**Files:**
- Create: `internal/audio/pcm.go`
- Test: `internal/audio/pcm_test.go`

- [ ] **Step 1: Write the failing test**

`internal/audio/pcm_test.go`:

```go
package audio

import (
	"math"
	"testing"
)

func TestS16LEToFloat32(t *testing.T) {
	// 0x0000 -> 0, 0x7FFF -> ~+1, 0x8000 -> -1
	pcm := []byte{0x00, 0x00, 0xFF, 0x7F, 0x00, 0x80}
	got := S16LEToFloat32(pcm)
	want := []float32{0, 32767.0 / 32768.0, -1}
	if len(got) != len(want) {
		t.Fatalf("len=%d want %d", len(got), len(want))
	}
	for i := range want {
		if math.Abs(float64(got[i]-want[i])) > 1e-6 {
			t.Fatalf("sample %d = %v want %v", i, got[i], want[i])
		}
	}
}

func TestS16LEToFloat32OddBytesIgnoresTrailing(t *testing.T) {
	if got := S16LEToFloat32([]byte{0x00, 0x00, 0x01}); len(got) != 1 {
		t.Fatalf("len=%d want 1 (trailing odd byte dropped)", len(got))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `CGO_ENABLED=0 go test ./internal/audio/ -run TestS16LEToFloat32 -v`
Expected: FAIL — `S16LEToFloat32 undefined`.

- [ ] **Step 3: Implement**

`internal/audio/pcm.go`:

```go
package audio

// S16LEToFloat32 converts little-endian signed 16-bit PCM to float32 samples
// in [-1, 1). A trailing odd byte is ignored. Mono is assumed; the wake-word
// path captures DefaultChannels (1) at DefaultSampleRate (16000).
func S16LEToFloat32(pcm []byte) []float32 {
	n := len(pcm) / 2
	out := make([]float32, n)
	for i := 0; i < n; i++ {
		v := int16(uint16(pcm[2*i]) | uint16(pcm[2*i+1])<<8)
		out[i] = float32(v) / 32768
	}
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `CGO_ENABLED=0 go test ./internal/audio/ -run TestS16LEToFloat32 -v`
Expected: PASS.

- [ ] **Step 5: Lint + commit**

```bash
golangci-lint run ./internal/audio/
git add internal/audio/pcm.go internal/audio/pcm_test.go
git commit -m "feat(audio): S16LEToFloat32 helper for the wake-word detector"
```

---

## Task 3: Wake-word detector core — P2.2

**Files:**
- Create: `internal/wakeword/detector.go`, `internal/wakeword/doc.go`
- Test: `internal/wakeword/detector_test.go`

The detector mirrors the validated spike pipeline (`cmd/wakeword-spike/main.go`) but as a reusable streaming component. Key correctness rule from the findings: feed `melspectrogram` a **continuous** audio stream so it yields 8 mel frames per 80 ms hop (the spike's isolated-chunk shortcut yielded ~5 and is wrong for scoring), and apply openWakeWord normalization `mel = mel/10 + 2` before the embedding model.

- [ ] **Step 1: Write the failing tests**

`internal/wakeword/detector_test.go`:

```go
package wakeword

import (
	"os"
	"testing"
)

// modelsDir / libPath come from env so CI without the ORT .so can skip.
func testPaths(t *testing.T) (lib, models string) {
	lib = os.Getenv("ONNXRUNTIME_LIB")
	models = os.Getenv("WAKEWORD_MODELS")
	if lib == "" || models == "" {
		t.Skip("set ONNXRUNTIME_LIB and WAKEWORD_MODELS to run wakeword tests")
	}
	return lib, models
}

func TestDetectorPositiveClipFires(t *testing.T) {
	lib, models := testPaths(t)
	d, err := New(Config{
		LibraryPath: lib,
		MelModel:    models + "/melspectrogram.onnx",
		EmbModel:    models + "/embedding_model.onnx",
		WakeModel:   models + "/hey_bmo.onnx",
		Threshold:   0.5,
		Threads:     2,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer d.Close()

	pcm := mustReadWAV(t, models+"/testdata/hey_jarvis_positive.wav") // 16k mono S16LE
	fired := false
	for _, frame := range chunk(pcm, 1280*2) { // 80ms S16LE byte chunks
		for _, det := range d.Write(frame) {
			if det.Score >= 0.5 {
				fired = true
			}
		}
	}
	if !fired {
		t.Fatalf("expected wake on positive clip")
	}
}

func TestDetectorSilenceDoesNotFire(t *testing.T) {
	lib, models := testPaths(t)
	d, err := New(Config{
		LibraryPath: lib, MelModel: models + "/melspectrogram.onnx",
		EmbModel: models + "/embedding_model.onnx", WakeModel: models + "/hey_bmo.onnx",
		Threshold: 0.5, Threads: 2,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer d.Close()
	silence := make([]byte, 16000*2*3) // 3s of silence
	for _, frame := range chunk(silence, 1280*2) {
		for _, det := range d.Write(frame) {
			if det.Score >= 0.5 {
				t.Fatalf("false trigger on silence: %v", det.Score)
			}
		}
	}
}
```

Add small helpers `chunk(b []byte, n int) [][]byte` and `mustReadWAV` (reuse `audio.DecodeWAV`) at the bottom of the test file. Obtain `testdata/hey_jarvis_positive.wav`: a "hey jarvis" sample (openWakeWord ships test audio; or record one and resample to 16k mono S16LE with `ffmpeg -i in.wav -ar 16000 -ac 1 -f s16le ...` wrapped in a WAV). Commit it under `internal/wakeword/testdata/`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `CGO_ENABLED=1 go test ./internal/wakeword/ -run TestDetector -v`
Expected: FAIL — package/`New` undefined (or skip if env unset; set env to actually drive this task).

- [ ] **Step 3: Implement the detector**

`internal/wakeword/detector.go`:

```go
// Package wakeword runs the openWakeWord ONNX pipeline (melspectrogram ->
// speech embedding -> wake classifier) on a 16 kHz mono stream and reports
// detections. Feasibility and budget: see
// docs/superpowers/2026-06-19-p2.0-wakeword-feasibility-findings.md.
package wakeword

import (
	"fmt"
	"sync"

	"github.com/carroarmato0/nextui-bmo/internal/audio"
	ort "github.com/yalue/onnxruntime_go"
)

const (
	hopSamples  = 1280 // 80 ms at 16 kHz
	melBins     = 32
	melPerHop   = 8 // mel frames produced per hop in a continuous stream
	embWindow   = 76
	embStride   = 8
	classWindow = 16
	embDim      = 96
	// melContext is extra trailing audio fed to the melspec model each hop so
	// the windowed transform produces melPerHop full frames (tuned in tests).
	melContext = 4800 // 300 ms; trim leading frames, keep the last melPerHop
)

type Config struct {
	LibraryPath string
	MelModel    string
	EmbModel    string
	WakeModel   string
	Threshold   float64 // default 0.5 if <= 0
	Threads     int     // default 2 if <= 0 (never 4 — see findings)
	// RefractorySteps suppresses re-firing for N hops after a detection
	// (debounce); default 12 (~1 s).
	RefractorySteps int
}

type Detection struct {
	Score float64
}

type Detector struct {
	mu                          sync.Mutex
	mel, emb, cls               *ort.DynamicAdvancedSession
	threshold                   float64
	refractory, sinceFired      int
	raw                         []float32 // rolling audio (>= hop+melContext)
	pending                     []float32 // not yet hopped
	melBuf                      []float32
	melFrames, framesSinceEmb   int
	embBuf                      []float32
	embCount                    int
	initOnce                    sync.Once
}

var envOnce sync.Once

// New loads the three models. The ORT environment is process-global and
// initialized once; LibraryPath must be consistent across detectors.
func New(c Config) (*Detector, error) { /* set defaults; ort.SetSharedLibraryPath + InitializeEnvironment under envOnce; build 3 DynamicAdvancedSessions with Threads via SessionOptions.SetIntra/InterOpNumThreads; see cmd/wakeword-spike/main.go mustSession */ }

// Write appends S16LE mono 16 kHz PCM and returns any detections produced by
// the newly completed 80 ms hops. Safe for one producer goroutine.
func (d *Detector) Write(pcm []byte) []Detection {
	samples := audio.S16LEToFloat32(pcm)
	d.mu.Lock()
	defer d.mu.Unlock()
	d.pending = append(d.pending, samples...)
	var out []Detection
	for len(d.pending) >= hopSamples {
		hop := d.pending[:hopSamples]
		d.pending = d.pending[hopSamples:]
		if det, ok := d.step(hop); ok {
			out = append(out, det)
		}
	}
	return out
}

// step runs one 80 ms hop through the pipeline and returns a Detection when the
// score crosses threshold and we are past the refractory window.
func (d *Detector) step(hop []float32) (Detection, bool) {
	d.raw = append(d.raw, hop...)
	if max := hopSamples + melContext; len(d.raw) > max {
		d.raw = d.raw[len(d.raw)-max:]
	}
	// melspec over the rolling window; keep only the last melPerHop frames.
	frames := d.runMel(d.raw)            // [][melBins], normalized mel/10+2
	newFrames := frames
	if len(frames) > melPerHop {
		newFrames = frames[len(frames)-melPerHop:]
	}
	for _, f := range newFrames {
		d.melBuf = append(d.melBuf, f...)
	}
	d.melFrames += len(newFrames)
	d.framesSinceEmb += len(newFrames)
	// embeddings every embStride frames once a full window exists
	for d.melFrames >= embWindow && d.framesSinceEmb >= embStride {
		start := (d.melFrames - embWindow) * melBins
		d.embBuf = append(d.embBuf, d.runEmb(d.melBuf[start:start+embWindow*melBins])...)
		d.embCount++
		d.framesSinceEmb -= embStride
		d.trimMel()
	}
	if d.sinceFired < d.refractory {
		d.sinceFired++
	}
	if d.embCount >= classWindow {
		start := (d.embCount - classWindow) * embDim
		score := d.runCls(d.embBuf[start : start+classWindow*embDim])
		d.trimEmb()
		if score >= d.threshold && d.sinceFired >= d.refractory {
			d.sinceFired = 0
			return Detection{Score: score}, true
		}
	}
	return Detection{}, false
}

// runMel/runEmb/runCls wrap one session.Run each (nil auto-allocated output),
// type-asserting *ort.Tensor[float32] and copying GetData(). runMel applies the
// mel/10+2 normalization and reshapes [1,1,frames,32] into [][melBins].
// trimMel/trimEmb bound the buffers (see cmd/wakeword-spike/main.go).

func (d *Detector) Close() error { /* Destroy 3 sessions; do NOT DestroyEnvironment (process-global, other detectors may exist) */ }
```

Fill `New`, `runMel`, `runEmb`, `runCls`, `trimMel`, `trimEmb` by lifting the validated bodies from `cmd/wakeword-spike/main.go` (the `step`/`mustSession` code there is the reference implementation). The only new logic vs. the spike is the rolling `melContext` window + `melPerHop` trimming for correct stream alignment.

- [ ] **Step 4: Run tests; tune `melContext` until positive fires and silence does not**

Run: `ONNXRUNTIME_LIB=... WAKEWORD_MODELS=... CGO_ENABLED=1 go test ./internal/wakeword/ -run TestDetector -v`
Expected: both PASS. If the positive clip does not fire, increase `melContext` / verify normalization and the 8-frames-per-hop alignment against openWakeWord's reference output.

- [ ] **Step 5: Lint + commit**

```bash
golangci-lint run ./internal/wakeword/
git add internal/wakeword/ internal/wakeword/testdata/
git commit -m "feat(wakeword): streaming openWakeWord detector (melspec->embedding->classifier)"
```

---

## Task 4: Detector benchmark guard (budget regression)

**Files:**
- Test: `internal/wakeword/detector_test.go` (add benchmark)

- [ ] **Step 1: Add a benchmark mirroring the spike measurement**

```go
func BenchmarkDetectorStep(b *testing.B) {
	lib := os.Getenv("ONNXRUNTIME_LIB"); models := os.Getenv("WAKEWORD_MODELS")
	if lib == "" || models == "" { b.Skip("env not set") }
	d, err := New(Config{LibraryPath: lib, MelModel: models+"/melspectrogram.onnx",
		EmbModel: models+"/embedding_model.onnx", WakeModel: models+"/hey_bmo.onnx", Threads: 2})
	if err != nil { b.Fatal(err) }
	defer d.Close()
	frame := make([]byte, hopSamples*2)
	b.ResetTimer()
	for i := 0; i < b.N; i++ { d.Write(frame) }
}
```

- [ ] **Step 2: Run on host to confirm sane numbers**

Run: `ONNXRUNTIME_LIB=... WAKEWORD_MODELS=... CGO_ENABLED=1 go test ./internal/wakeword/ -bench BenchmarkDetectorStep -run x -benchtime 200x`
Expected: completes; ns/op far below 80 ms (host) — the on-device budget is the findings doc's ~19 ms/frame @ 2 threads.

- [ ] **Step 3: Commit**

```bash
git add internal/wakeword/detector_test.go
git commit -m "test(wakeword): per-step benchmark guard"
```

---

## Task 5: CPU governor control — P2.7

**Files:**
- Create: `internal/power/governor.go`
- Test: `internal/power/governor_test.go`

Resolved open question: drive `/sys/devices/system/cpu/cpu*/cpufreq/scaling_governor` directly. Confirmed on device: default `schedutil`, `performance` available. Non-writable (desktop / manual launch) → log + no-op.

- [ ] **Step 1: Write the failing test (fake sysfs root)**

`internal/power/governor_test.go`:

```go
package power

import (
	"os"
	"path/filepath"
	"testing"
)

func writeGov(t *testing.T, root, cpu, val string) string {
	dir := filepath.Join(root, cpu, "cpufreq")
	if err := os.MkdirAll(dir, 0o755); err != nil { t.Fatal(err) }
	p := filepath.Join(dir, "scaling_governor")
	if err := os.WriteFile(p, []byte(val+"\n"), 0o644); err != nil { t.Fatal(err) }
	return p
}

func TestGovernorRequestAndRestore(t *testing.T) {
	root := t.TempDir()
	p0 := writeGov(t, root, "cpu0", "schedutil")
	p1 := writeGov(t, root, "cpu1", "schedutil")
	g := &Governor{Root: root, Desired: "performance"}
	if err := g.Request(); err != nil { t.Fatalf("request: %v", err) }
	for _, p := range []string{p0, p1} {
		if b, _ := os.ReadFile(p); string(b) != "performance\n" {
			t.Fatalf("%s = %q want performance", p, b)
		}
	}
	if err := g.Restore(); err != nil { t.Fatalf("restore: %v", err) }
	for _, p := range []string{p0, p1} {
		if b, _ := os.ReadFile(p); string(b) != "schedutil\n" {
			t.Fatalf("%s = %q want restored schedutil", p, b)
		}
	}
}

func TestGovernorRequestRefcounts(t *testing.T) {
	root := t.TempDir()
	p0 := writeGov(t, root, "cpu0", "schedutil")
	g := &Governor{Root: root, Desired: "performance"}
	_ = g.Request()
	_ = g.Request() // nested burst
	_ = g.Restore() // still in a burst
	if b, _ := os.ReadFile(p0); string(b) != "performance\n" {
		t.Fatalf("released too early: %q", b)
	}
	_ = g.Restore()
	if b, _ := os.ReadFile(p0); string(b) != "schedutil\n" {
		t.Fatalf("not restored after final release: %q", b)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `CGO_ENABLED=0 go test ./internal/power/ -v`
Expected: FAIL — package/`Governor` undefined.

- [ ] **Step 3: Implement**

`internal/power/governor.go`:

```go
// Package power requests the performance CPU governor during STT/TTS bursts
// and restores the prior governor afterward. Always-on use is avoided: the
// wake-word detector runs at the default governor (see Phase 2 spec P2.7).
package power

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type Governor struct {
	Root    string // default "/sys/devices/system/cpu"
	Desired string // default "performance"
	Logf    func(string, ...any)

	mu    sync.Mutex
	depth int
	prev  map[string]string // path -> original governor
}

func (g *Governor) root() string {
	if g.Root == "" { return "/sys/devices/system/cpu" }
	return g.Root
}
func (g *Governor) desired() string {
	if g.Desired == "" { return "performance" }
	return g.Desired
}

func (g *Governor) paths() []string {
	matches, _ := filepath.Glob(filepath.Join(g.root(), "cpu[0-9]*", "cpufreq", "scaling_governor"))
	sort.Strings(matches)
	return matches
}

// Request switches all CPUs to the desired governor, recording originals on the
// first (outermost) call. Refcounted so overlapping bursts nest safely.
func (g *Governor) Request() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.depth++
	if g.depth > 1 { return nil }
	g.prev = map[string]string{}
	for _, p := range g.paths() {
		cur, err := os.ReadFile(p)
		if err != nil { g.warn("read %s: %v", p, err); continue }
		g.prev[p] = strings.TrimSpace(string(cur))
		if err := os.WriteFile(p, []byte(g.desired()), 0o644); err != nil {
			g.warn("set %s: %v", p, err) // non-writable desktop: no-op
		}
	}
	return nil
}

// Restore reverts to the recorded governors when the outermost burst ends.
func (g *Governor) Restore() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.depth == 0 { return nil }
	g.depth--
	if g.depth > 0 { return nil }
	for p, v := range g.prev {
		if err := os.WriteFile(p, []byte(v), 0o644); err != nil {
			g.warn("restore %s: %v", p, err)
		}
	}
	g.prev = nil
	return nil
}

func (g *Governor) warn(f string, a ...any) { if g.Logf != nil { g.Logf(f, a...) } }
```

- [ ] **Step 4: Run to verify pass**

Run: `CGO_ENABLED=0 go test ./internal/power/ -v`
Expected: PASS.

- [ ] **Step 5: Lint + commit**

```bash
golangci-lint run ./internal/power/
git add internal/power/
git commit -m "feat(power): refcounted performance-governor request/restore"
```

---

## Task 6: Config fields — part of P2.8

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/config/config_test.go`:

```go
func TestContinuedConversationNormalizeDefaults(t *testing.T) {
	c := Config{ContinuedConversation: "BOGUS"}
	c.Normalize()
	if c.ContinuedConversation != ContinuedConvoOff {
		t.Fatalf("got %q want %q", c.ContinuedConversation, ContinuedConvoOff)
	}
}

func TestWakeWordValidatesTrigger(t *testing.T) {
	c := Default()
	c.Mode = ModeAI
	c.WakeWordEnabled = true
	c.InputTrigger = InputTriggerWakeWord
	if err := c.Validate(); err != nil {
		t.Fatalf("valid wake-word config rejected: %v", err)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `CGO_ENABLED=0 go test ./internal/config/ -run 'ContinuedConversation|WakeWord' -v`
Expected: FAIL — undefined identifiers.

- [ ] **Step 3: Implement**

In `internal/config/config.go`: add to the `Config` struct (near `InputTrigger`):

```go
	WakeWordEnabled       bool   `json:"wake_word_enabled,omitempty"`
	ContinuedConversation string `json:"continued_conversation,omitempty"`
```

Add constants near `InputTriggerWakeWord`:

```go
const (
	ContinuedConvoOff   = "off"
	ContinuedConvoShort = "short" // ~8 s window
	ContinuedConvoLong  = "long"  // ~20 s window (two-BMO conversations)
)
```

In `Normalize()`, after the existing `InputTrigger` default block, add:

```go
	switch c.ContinuedConversation {
	case ContinuedConvoOff, ContinuedConvoShort, ContinuedConvoLong:
	default:
		c.ContinuedConversation = ContinuedConvoOff
	}
```

No new `Validate()` rule is required (any combination is valid); `TestWakeWordValidatesTrigger` just guards that the existing trigger validation already accepts `InputTriggerWakeWord` (it does, per config.go:304).

- [ ] **Step 4: Run to verify pass**

Run: `CGO_ENABLED=0 go test ./internal/config/ -v`
Expected: PASS.

- [ ] **Step 5: Lint + commit**

```bash
golangci-lint run ./internal/config/
git add internal/config/
git commit -m "feat(config): WakeWordEnabled + ContinuedConversation fields"
```

---

## Task 7: Settings UI rows — P2.8

**Files:**
- Modify: `internal/ui/settings_menu.go`
- Test: `internal/ui/settings_menu_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/ui/settings_menu_test.go` (follow the existing AI-toggle test pattern in this file for focusing a row by Code and asserting the resulting cfg):

```go
func TestSettingsWakeWordToggle(t *testing.T) {
	cfg := config.Default()
	cfg.Mode = config.ModeAI
	m := NewSettingsMenu(cfg)
	focusByCode(t, m, "wake_word") // reuse the file's existing focus helper
	if err := m.ToggleFocused(); err != nil {
		t.Fatalf("toggle: %v", err)
	}
	if !m.Config().WakeWordEnabled {
		t.Fatalf("wake word not enabled after toggle")
	}
}

func TestSettingsContinuedConvoCycles(t *testing.T) {
	cfg := config.Default()
	cfg.Mode = config.ModeAI
	cfg.WakeWordEnabled = true
	m := NewSettingsMenu(cfg)
	focusByCode(t, m, "continued_convo")
	_ = m.Cycle(1)
	if m.Config().ContinuedConversation == config.ContinuedConvoOff {
		t.Fatalf("continued conversation did not cycle off->short")
	}
}
```

If no `focusByCode` helper exists, look at how the existing toggle tests focus a slot (they likely call `Move` until `Overlay()` focus matches the Code) and reuse that exact mechanism.

- [ ] **Step 2: Run to verify failure**

Run: `CGO_ENABLED=0 go test ./internal/ui/ -run 'WakeWord|ContinuedConvo' -v`
Expected: FAIL — rows absent.

- [ ] **Step 3: Implement**

In `slots()` (internal/ui/settings_menu.go), add two rows in the AI-only group (e.g. just after `proactive_talk`). WAKE WORD is a plain AI toggle; CONTINUED CONVO is an AI cycle hidden unless wake word is on:

```go
		aiToggle("wake_word", "WAKE WORD: "+onOff(m.cfg.WakeWordEnabled), m.cfg.WakeWordEnabled),
		func() settingsSlot {
			s := aiCycle("continued_convo", "CONTINUED CONVO: "+strings.ToUpper(m.cfg.ContinuedConversation))
			if !m.cfg.WakeWordEnabled {
				s.item.Hidden = true
				s.navigable = false
			}
			return s
		}(),
```

Bump `const settingsSlotCount` from 19 to **21**.

In `ToggleFocused()`, add a case (matching the existing `aware_*` toggle cases) that flips `m.cfg.WakeWordEnabled`. Also: when wake word turns **on**, set `m.cfg.InputTrigger = config.InputTriggerWakeWord`; when it turns **off**, set it back to `config.InputTriggerPTT` (so the input model has a single source of truth). When `m.cfg.ContinuedConversation == ""`, initialize it to `config.ContinuedConvoShort` on first enable.

In `Cycle()`, add a case for `"continued_convo"` cycling `off → short → long → off` (respect `delta` sign like other cycle rows; reuse the existing cycle-order helper pattern, e.g. a local `[]string{off, short, long}`).

- [ ] **Step 4: Run to verify pass**

Run: `CGO_ENABLED=0 go test ./internal/ui/ -v`
Expected: PASS (new + existing settings tests; the slot-count change must not break existing index-based tests — update any fixed indices they assert).

- [ ] **Step 5: Lint + commit**

```bash
golangci-lint run ./internal/ui/
git add internal/ui/
git commit -m "feat(ui): WAKE WORD + CONTINUED CONVO settings rows"
```

---

## Task 8: Wake-word runtime wiring — P2.3 / P2.4 / P2.5 / P2.6

**Files:**
- Create: `cmd/bmo-pak/wakeword.go`, `cmd/bmo-pak/wakeword_test.go`
- Modify: `cmd/bmo-pak/main.go`

This is the integration task. `startWakeWord` mirrors `startPushToTalk` (cmd/bmo-pak/ptt_shared.go) and reuses its building blocks: an `input.Buffer` to accumulate the utterance, `machine.Transition`, and `pipeline.ProcessBatch`. To keep it unit-testable, factor the decision logic into a `wakeController` that depends on interfaces, not the concrete detector/pipeline.

- [ ] **Step 1: Write the failing tests (controller logic, no ONNX)**

`cmd/bmo-pak/wakeword_test.go` — drive a `wakeController` with a fake clock and injected events:

```go
package main

import (
	"testing"
	"time"
)

func TestWakeIgnoredWhileSpeaking(t *testing.T) {
	c := newTestController() // speaking=true, idle otherwise
	c.speaking = true
	if c.onDetection(c.now) {
		t.Fatalf("must not trigger while BMO is speaking")
	}
}

func TestWakeIgnoredDuringGuardTail(t *testing.T) {
	c := newTestController()
	c.speaking = false
	c.speechEndedAt = c.now // just stopped speaking
	if c.onDetection(c.now.Add(200 * time.Millisecond)) {
		t.Fatalf("must not trigger within guard tail after speech")
	}
	if !c.onDetection(c.now.Add(c.guardTail + time.Millisecond)) {
		t.Fatalf("should trigger after guard tail elapses")
	}
}

func TestContinuedConversationReopensWindow(t *testing.T) {
	c := newTestController()
	c.continuedWindow = 8 * time.Second
	c.onReplyFinished(c.now)
	if !c.windowOpen(c.now.Add(3 * time.Second)) {
		t.Fatalf("follow-up window should be open")
	}
	if c.windowOpen(c.now.Add(9 * time.Second)) {
		t.Fatalf("window should have expired")
	}
}

func TestFollowUpCapBacksOff(t *testing.T) {
	c := newTestController()
	c.continuedWindow = 8 * time.Second
	c.maxFollowUps = 2
	for i := 0; i < 2; i++ {
		c.onReplyFinished(c.now)
		c.startFollowUp(c.now)
	}
	c.onReplyFinished(c.now)
	if c.windowOpen(c.now.Add(time.Second)) {
		t.Fatalf("window must stay closed after max follow-ups")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `CGO_ENABLED=0 go test ./cmd/bmo-pak/ -run 'Wake|ContinuedConversation|FollowUp' -v`
Expected: FAIL — undefined `wakeController`. (Note: `cmd/bmo-pak` needs CGO for SDL elsewhere, but `go test` compiles the whole package; if SDL blocks compilation, keep `wakeController` in a file without SDL imports — it only needs `time` — and it will still compile under the package's normal CGO build.)

- [ ] **Step 3: Implement `wakeController` + `startWakeWord`**

`cmd/bmo-pak/wakeword.go`:

```go
package main

import (
	"context"
	"time"

	"github.com/carroarmato0/nextui-bmo/internal/assistant"
	"github.com/carroarmato0/nextui-bmo/internal/audio"
	"github.com/carroarmato0/nextui-bmo/internal/config"
	"github.com/carroarmato0/nextui-bmo/internal/hardware"
	"github.com/carroarmato0/nextui-bmo/internal/wakeword"
)

// wakeController holds the pure gating/window logic (testable without ONNX).
type wakeController struct {
	now             time.Time
	speaking        bool
	speechEndedAt   time.Time
	guardTail       time.Duration // ~600 ms
	continuedWindow time.Duration // 0 = continued conversation off
	windowUntil     time.Time
	maxFollowUps    int
	followUps       int
}

// onDetection reports whether a detection at time t should trigger capture:
// only when BMO is not speaking and the post-speech guard tail has elapsed.
func (c *wakeController) onDetection(t time.Time) bool {
	if c.speaking {
		return false
	}
	if !c.speechEndedAt.IsZero() && t.Sub(c.speechEndedAt) < c.guardTail {
		return false
	}
	return true
}

// onReplyFinished opens the continued-conversation follow-up window (unless the
// follow-up cap is reached or continued conversation is off).
func (c *wakeController) onReplyFinished(t time.Time) {
	c.speaking = false
	c.speechEndedAt = t
	if c.continuedWindow <= 0 || c.followUps >= c.maxFollowUps {
		c.windowUntil = time.Time{}
		return
	}
	c.windowUntil = t.Add(c.continuedWindow)
}

func (c *wakeController) windowOpen(t time.Time) bool {
	return !c.windowUntil.IsZero() && t.Before(c.windowUntil)
}

func (c *wakeController) startFollowUp(t time.Time) { c.followUps++; c.windowUntil = time.Time{} }
func (c *wakeController) resetFollowUps()           { c.followUps = 0 }
```

Then `startWakeWord(ctx, logger, machine, cfg, profile, router, pipeline, gov, assets)` (mirror `startPushToTalk`'s signature/guards):
- Return early no-op unless `cfg.UsesAI() && cfg.WakeWordEnabled && cfg.InputTrigger == config.InputTriggerWakeWord`.
- Build the detector: `wakeword.New(wakeword.Config{LibraryPath: assets.ORTLib, MelModel: ..., EmbModel: ..., WakeModel: ..., Threshold: 0.5, Threads: 2})`. On error, `logger.Warnf("wake word disabled: %v")` and no-op.
- `sub, cancel := router.Subscribe()`. One goroutine reads `sub`; **only** feed the detector when `machine.State() == assistant.StateIdle` (P2.2: idle + enabled) **and** the controller's `onDetection` gate passes; otherwise drain without detecting (and reset detector buffers on state changes to avoid stale context).
- On a detection: `pipeline.InterruptSpeech()` is unnecessary (we only run while idle); drive the PTT-equivalent path: `machine.Transition(assistant.EventListen)`, accumulate the following ~N seconds of `sub` batches into an `input.Buffer` until a level-based end-of-speech (use `router.Levels()` / `audio.PCMLevelS16LE` for VAD) or a hard max-duration cap (P2.4), then `machine.Transition(assistant.EventRest)` and send to `pipeline.ProcessBatch(ctx, utterance)`.
- Set `c.speaking` from machine state transitions (speaking on EventSpeak, cleared via `onReplyFinished` when playback returns). Wrap STT/TTS bursts with `gov.Request()` / `defer gov.Restore()`.
- After `ProcessBatch` returns (reply finished), call `onReplyFinished(time.Now())`; if `windowOpen`, loop back into capture (a follow-up) without requiring a new wake; `startFollowUp` on each; `resetFollowUps` when the window expires to idle.

Keep all timing constants named: `guardTail = 600 * time.Millisecond`, `maxCapture = 10 * time.Second`, `maxFollowUps = 6`, window from `cfg.ContinuedConversation` (`short`→8s, `long`→20s, `off`→0).

- [ ] **Step 4: Run controller tests to verify pass**

Run: `CGO_ENABLED=0 go test ./cmd/bmo-pak/ -run 'Wake|ContinuedConversation|FollowUp' -v`
Expected: PASS.

- [ ] **Step 5: Wire into `main.go`**

In `cmd/bmo-pak/main.go`, near the `startPushToTalk` call (main.go:359), construct the asset paths (next to the existing pak asset resolution), build a `power.Governor`, and call `startWakeWord(...)`; keep its returned stop func alongside `stopPTT`. Pass the same `gov` into the existing PTT/voice burst path if cheap, else leave PTT unchanged. Ensure only one of PTT/wake-word arms based on `cfg.InputTrigger`.

- [ ] **Step 6: Build + full test**

Run: `CGO_ENABLED=1 go build ./... && CGO_ENABLED=1 go test ./cmd/bmo-pak/ ./internal/... 2>&1 | grep -vE '\[no test files\]|^ok'`
Expected: empty (all pass).

- [ ] **Step 7: Lint + commit**

```bash
golangci-lint run ./...
git add cmd/bmo-pak/
git commit -m "feat(bmo-pak): wake-word trigger, self-trigger gating, continued conversation"
```

---

## Task 9: Ship ORT runtime + models as pak assets

**Files:**
- Modify: `scripts/release.sh`

- [ ] **Step 1: Add the ORT `.so` + models to the packaged pak**

In `build_platform()` (after the SDL2 copy), copy the aarch64 `libonnxruntime.so` into `lib/${platform}/libonnxruntime.so`, and copy `melspectrogram.onnx`, `embedding_model.onnx`, `hey_bmo.onnx` into the pak's `assets/wakeword/`. Source the `.so` and models from a committed `third_party/onnxruntime/` + `assets/wakeword/` (add them to the repo, or document a fetch step in the release script that downloads the pinned versions from the findings doc). Mirror exactly how `SDL2_SO` is located and copied.

- [ ] **Step 2: Verify the packaged tree**

Run: `./scripts/release.sh` then inspect `dist/.../BMO.pak/lib/tg5040/libonnxruntime.so` and `dist/.../BMO.pak/assets/wakeword/*.onnx` exist.
Expected: all present; pak size increase ≈ 23 MB.

- [ ] **Step 3: Commit**

```bash
git add scripts/release.sh assets/wakeword/ third_party/onnxruntime/
git commit -m "build: bundle onnxruntime + wake-word models as pak assets"
```

---

## Task 10: On-device verification — P2.9 (manual)

- [ ] Deploy (`./scripts/deploy.sh`), enable WAKE WORD in settings, and confirm via `./scripts/debug-logs.sh`:
  - detector loads (no asset/`.so` errors), runs only while idle;
  - "Hey BMO" (stock hey-jarvis) triggers a capture→reply within budget;
  - BMO does **not** wake on its own TTS (self-trigger gating + guard tail);
  - with CONTINUED CONVO on, a follow-up within the window is accepted without re-waking; window expires; max-follow-up cap holds;
  - governor flips to `performance` during a burst and restores after (`adb shell cat /sys/devices/system/cpu/cpu0/cpufreq/scaling_governor`);
  - RSS in budget (`adb shell cat /proc/<pid>/status | grep VmRSS`), no OOM over a multi-minute session.
- [ ] Record results in a short addendum to the findings doc.

---

## Docs (fold into the relevant task commits)

- `README.md` / `docs/MODDING.md`: document the WAKE WORD + CONTINUED CONVO settings, the always-on-mic battery trade-off (default off), and that the shipped model is stock "hey jarvis" pending a trained "Hey BMO" model (follow-up project, P2.2 note).

---

## Self-Review

**Spec coverage:** P2.1 fan-out → Task 1. P2.2 detector → Tasks 3–4. P2.3 trigger (same path as PTT) → Task 8 step 3/5. P2.4 listening window (VAD end + max cap) → Task 8. P2.5 self-trigger gating (speaking + guard tail) → Task 8 (`wakeController.onDetection`) + tests. P2.6 continued conversation (window + max-follow-up cap) → Task 8 (`onReplyFinished`/`windowOpen`/`startFollowUp`) + tests. P2.7 governor → Task 5. P2.8 config + settings → Tasks 6–7. P2.9 tests → unit tests across tasks + Task 10 on-device. Open questions resolved: governor via `/sys` (Task 5); ship `.so`+models as assets (Task 9, findings doc).

**Placeholder scan:** New packages (`wakeword`, `power`) and the fan-out have full code or explicit lifts from the committed spike (`cmd/wakeword-spike/main.go`). Task 8's `startWakeWord` body is described as concrete steps over verified call sites rather than a full inline rewrite, because it threads through existing `main.go` wiring (`startPushToTalk` at main.go:359, `input.Buffer`, `machine.Transition`, `pipeline.ProcessBatch`) that must be matched in-place; the pure logic (`wakeController`) is fully specified and TDD-tested.

**Type consistency:** `Detector.Write([]byte) []Detection`, `Detection.Score float64`, `wakeword.Config` fields, `Governor.Request/Restore`, config `WakeWordEnabled`/`ContinuedConversation` + `ContinuedConvo{Off,Short,Long}`, and settings codes `wake_word`/`continued_convo` are used consistently across tasks.
```
