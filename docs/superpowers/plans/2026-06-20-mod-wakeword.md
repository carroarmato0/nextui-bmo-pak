# Mod-supplied Wake Word Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a mod ship `wakeword/wake.onnx` to replace BMO's "Hey BMO" classifier, validated against the detector contract and applied live when the mod is selected.

**Architecture:** A pure `resolveWakeModel` extracts the active mod's `wakeword/wake.onnx` to a temp file (uniform for directory and zip mods); `buildWakeAssets` composes that with the pak's base models and validates the candidate via the existing `wakeword.ValidateClassifier`, silently falling back to the pak default on any problem. `run()` drives wake setup through a `restartWake` closure (defined where `gov`/`pakDir` are in scope) called at startup and from the existing `reloadMod` mod-switch handler, rebuilding the detector synchronously.

**Tech Stack:** Go 1.25, `internal/wakeword` (onnxruntime contract), `internal/mod` (mod FS), `cmd/bmo-pak` wake wiring. Tests: `CGO_ENABLED=1 go test`. Full build needs CGO (SDL).

**Spec:** `docs/superpowers/specs/2026-06-20-mod-wakeword-design.md`

## Verified anchors (serena + reads)

- `wakeword.New` is built in exactly one place: `startWakeWord` (`cmd/bmo-pak/wakeword.go:145`). No change to `startWakeWord` is required.
- `wakeword.InitEnv` is idempotent and `ValidateClassifier`/`InitEnv` are real, tested APIs (`internal/wakeword/contract.go`, used by `cmd/wakeword-eval`).
- The wake wiring lives in `run()` (`cmd/bmo-pak/main.go`): `var stopWake func()` at line 310; `wakeAssets`/`gov`/`pakDir` built locally at lines 363-371 inside `if cfg.UsesAI() && audioSession != nil`; `reloadMod := func(id string)` top-level closure at line 419 (runs on the main goroutine).
- `mod.Mod` has `ID string` and `FS fs.FS` (rooted at mod contents; `wakeword/wake.onnx` sits beside `faces/`, `audio/`). `reloadMod` already calls `active.Open(...)` before reading `active.FS`.

## File Structure

- `cmd/bmo-pak/modwake.go` (new) — `resolveWakeModel` (pure extraction) + `buildWakeAssets` (compose + validate + fallback + log).
- `cmd/bmo-pak/modwake_test.go` (new) — unit tests for both (the ORT happy-path is verified on-device; dev tests cover resolution + fallback).
- `cmd/bmo-pak/main.go` — replace the inline wake setup with a `restartWake` closure; call it at startup and in `reloadMod`.
- `docs/MODDING.md`, `README.md`, `training/wakeword/README.md` — document the convention.

---

## Task 1: `resolveWakeModel` — extract a mod's wake model (pure)

**Files:**
- Create: `cmd/bmo-pak/modwake.go`
- Test: `cmd/bmo-pak/modwake_test.go`

- [ ] **Step 1: Write the failing test**

Create `cmd/bmo-pak/modwake_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestResolveWakeModelExtractsCustom(t *testing.T) {
	want := []byte("fake-onnx-bytes")
	modFS := fstest.MapFS{"wakeword/wake.onnx": &fstest.MapFile{Data: want}}
	tmp := t.TempDir()

	path, custom, err := resolveWakeModel(modFS, "evil-bmo", "/pak/hey_bmo.onnx", tmp)
	if err != nil {
		t.Fatalf("resolveWakeModel: %v", err)
	}
	if !custom {
		t.Fatal("expected custom=true when wakeword/wake.onnx is present")
	}
	if path != filepath.Join(tmp, "wake-evil-bmo.onnx") {
		t.Fatalf("path = %q, want tmp/wake-evil-bmo.onnx", path)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read extracted: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("extracted bytes = %q, want %q", got, want)
	}
}

func TestResolveWakeModelFallsBackWhenAbsent(t *testing.T) {
	modFS := fstest.MapFS{"faces/neutral.svg": &fstest.MapFile{Data: []byte("x")}}
	path, custom, err := resolveWakeModel(modFS, "plain", "/pak/hey_bmo.onnx", t.TempDir())
	if err != nil {
		t.Fatalf("resolveWakeModel: %v", err)
	}
	if custom {
		t.Fatal("expected custom=false when no wakeword/wake.onnx")
	}
	if path != "/pak/hey_bmo.onnx" {
		t.Fatalf("path = %q, want default", path)
	}
}

func TestResolveWakeModelNilFS(t *testing.T) {
	path, custom, err := resolveWakeModel(nil, "x", "/pak/hey_bmo.onnx", t.TempDir())
	if err != nil || custom || path != "/pak/hey_bmo.onnx" {
		t.Fatalf("nil FS: got (%q,%v,%v), want default", path, custom, err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `CGO_ENABLED=1 go test ./cmd/bmo-pak/ -run TestResolveWakeModel -v`
Expected: FAIL — `resolveWakeModel` undefined.

- [ ] **Step 3: Implement**

Create `cmd/bmo-pak/modwake.go` (only the imports `resolveWakeModel` needs — Task 2 adds the `mod`/`wakeword` imports when `buildWakeAssets` lands):

```go
package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// modWakeModelName is the conventional path, within a mod's FS, of a custom
// wake-word classifier that replaces the stock "Hey BMO" model.
const modWakeModelName = "wakeword/wake.onnx"

// resolveWakeModel returns a filesystem path to the wake classifier to use: the
// active mod's wakeword/wake.onnx (extracted to tmpDir so onnxruntime can load
// it regardless of whether the mod is a directory or a zip) if present, else
// defaultPath. custom reports whether a mod-supplied model was extracted. A
// missing/unreadable mod file is not an error — it falls back to the default.
func resolveWakeModel(modFS fs.FS, modID, defaultPath, tmpDir string) (path string, custom bool, err error) {
	if modFS == nil {
		return defaultPath, false, nil
	}
	data, readErr := fs.ReadFile(modFS, modWakeModelName)
	if readErr != nil {
		return defaultPath, false, nil // absent/unreadable -> default
	}
	out := filepath.Join(tmpDir, "wake-"+modID+".onnx")
	if writeErr := os.WriteFile(out, data, 0o644); writeErr != nil {
		return defaultPath, false, fmt.Errorf("extract wake model for mod %q: %w", modID, writeErr)
	}
	return out, true, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `CGO_ENABLED=1 go test ./cmd/bmo-pak/ -run TestResolveWakeModel -v`
Expected: PASS

- [ ] **Step 5: Lint**

Run: `golangci-lint run ./cmd/bmo-pak/...`
Expected: no findings.

- [ ] **Step 6: Commit**

```bash
git add cmd/bmo-pak/modwake.go cmd/bmo-pak/modwake_test.go
git commit -m "feat(bmo-pak): resolveWakeModel extracts a mod's wakeword/wake.onnx"
```
No `Co-Authored-By` trailer.

---

## Task 2: `buildWakeAssets` — compose, validate, fall back

**Files:**
- Modify: `cmd/bmo-pak/modwake.go`
- Test: `cmd/bmo-pak/modwake_test.go`

- [ ] **Step 1: Write the failing test**

Add to `cmd/bmo-pak/modwake_test.go`:

```go
func TestBuildWakeAssetsNoCustomUsesDefault(t *testing.T) {
	m := mod.Mod{ID: "plain", FS: fstest.MapFS{"faces/x.svg": &fstest.MapFile{Data: []byte("x")}}}
	assets, cleanup := buildWakeAssets(m, "/pak", "tg5040", t.TempDir(), testLogger{})
	defer cleanup()
	if assets.WakeModel != filepath.Join("/pak", "assets", "wakeword", "hey_bmo.onnx") {
		t.Fatalf("WakeModel = %q, want pak default", assets.WakeModel)
	}
	if assets.MelModel == "" || assets.EmbModel == "" || assets.ORTLib == "" {
		t.Fatal("base asset paths must be populated from pakDir")
	}
}

func TestBuildWakeAssetsInvalidCustomFallsBackAndCleansUp(t *testing.T) {
	// A custom model is present, but ORT cannot validate it here (no real
	// runtime / bogus pakDir lib), so buildWakeAssets must fall back to the
	// default AND remove the extracted temp file.
	m := mod.Mod{ID: "evil", FS: fstest.MapFS{"wakeword/wake.onnx": &fstest.MapFile{Data: []byte("not-a-real-onnx")}}}
	tmp := t.TempDir()
	assets, cleanup := buildWakeAssets(m, "/pak", "tg5040", tmp, testLogger{})
	defer cleanup()
	if assets.WakeModel != filepath.Join("/pak", "assets", "wakeword", "hey_bmo.onnx") {
		t.Fatalf("WakeModel = %q, want pak default after invalid custom", assets.WakeModel)
	}
	if _, err := os.Stat(filepath.Join(tmp, "wake-evil.onnx")); !os.IsNotExist(err) {
		t.Fatalf("temp extracted model should have been cleaned up, stat err = %v", err)
	}
}
```

Add a tiny test logger to `cmd/bmo-pak/modwake_test.go` (matches the `pttLogger` interface — check `cmd/bmo-pak/ptt_shared.go` for the exact method set; it has at least `Infof`, `Warnf`, `Debugf` taking `(string, ...any)` / `(string, ...interface{})`):

```go
type testLogger struct{}

func (testLogger) Infof(string, ...any)  {}
func (testLogger) Warnf(string, ...any)  {}
func (testLogger) Debugf(string, ...any) {}
```

> Before writing this, open `cmd/bmo-pak/ptt_shared.go` and confirm the `pttLogger` interface method set and exact variadic type (`...any` vs `...interface{}`). Match it exactly; add any other methods the interface declares as no-ops. If `pttLogger` is already satisfied by an existing test helper in the package, reuse that instead of declaring `testLogger`.

- [ ] **Step 2: Run test to verify it fails**

Run: `CGO_ENABLED=1 go test ./cmd/bmo-pak/ -run TestBuildWakeAssets -v`
Expected: FAIL — `buildWakeAssets` undefined.

- [ ] **Step 3: Implement**

In `cmd/bmo-pak/modwake.go`, add the `mod` and `wakeword` imports:

```go
import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/carroarmato0/nextui-bmo/internal/mod"
	"github.com/carroarmato0/nextui-bmo/internal/wakeword"
)
```

Add:

```go
// buildWakeAssets locates the ONNX runtime library and base models in the pak,
// and resolves the wake classifier from the active mod (validated against the
// detector contract). On any problem — extraction failure, ORT init failure, or
// a model that fails the [_,16,96]->[_,1] contract — it logs and falls back to
// the pak's stock hey_bmo.onnx. The returned cleanup removes any extracted temp
// model; it is a no-op when the default is used. cleanup must be called after
// the detector built from these assets is closed.
func buildWakeAssets(activeMod mod.Mod, pakDir, platform, tmpDir string, logger pttLogger) (wakeAssets, func()) {
	assets := wakeAssets{
		ORTLib:   filepath.Join(pakDir, "lib", platform, "libonnxruntime.so"),
		MelModel: filepath.Join(pakDir, "assets", "wakeword", "melspectrogram.onnx"),
		EmbModel: filepath.Join(pakDir, "assets", "wakeword", "embedding_model.onnx"),
	}
	defaultWake := filepath.Join(pakDir, "assets", "wakeword", "hey_bmo.onnx")

	path, custom, err := resolveWakeModel(activeMod.FS, activeMod.ID, defaultWake, tmpDir)
	if err != nil {
		logger.Warnf("wake model: mod %q extraction failed: %v; model=default", activeMod.ID, err)
		assets.WakeModel = defaultWake
		return assets, func() {}
	}
	if !custom {
		logger.Infof("wake model: model=default")
		assets.WakeModel = defaultWake
		return assets, func() {}
	}

	cleanup := func() { _ = os.Remove(path) }
	if initErr := wakeword.InitEnv(assets.ORTLib); initErr != nil {
		logger.Warnf("wake model: ORT init failed (%v); mod %q ignored; model=default", initErr, activeMod.ID)
		cleanup()
		assets.WakeModel = defaultWake
		return assets, func() {}
	}
	if vErr := wakeword.ValidateClassifier(path); vErr != nil {
		logger.Warnf("wake model: mod %q model invalid (%v); model=default", activeMod.ID, vErr)
		cleanup()
		assets.WakeModel = defaultWake
		return assets, func() {}
	}
	logger.Infof("wake model: model=mod(%s)", activeMod.ID)
	assets.WakeModel = path
	return assets, cleanup
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `CGO_ENABLED=1 go test ./cmd/bmo-pak/ -run 'TestResolveWakeModel|TestBuildWakeAssets' -v`
Expected: PASS (the invalid-custom test exercises the ORT-init/validation fallback without a working runtime).

- [ ] **Step 5: Build + lint**

Run: `CGO_ENABLED=1 go build ./... && golangci-lint run ./cmd/bmo-pak/...`
Expected: build succeeds; no findings.

- [ ] **Step 6: Commit**

```bash
git add cmd/bmo-pak/modwake.go cmd/bmo-pak/modwake_test.go
git commit -m "feat(bmo-pak): buildWakeAssets validates mod wake model with fallback"
```
No `Co-Authored-By` trailer.

---

## Task 3: Wire `restartWake` into startup + mod switch (live swap)

**Files:**
- Modify: `cmd/bmo-pak/main.go` (`run()` — declarations ~line 310, wake setup ~lines 363-371, `reloadMod` ~line 458)

No new unit tests (main's `run()` is not unit-tested); verified by build + on-device. Read each anchor before editing; if it differs, report NEEDS_CONTEXT.

- [ ] **Step 1: Add top-level state for the rebuild closure**

In `run()`, next to `var stopWake func()` (line 310), add:

```go
	var stopWake func()
	var restartWake func(mod.Mod)
	var wakeCleanup func()
```

- [ ] **Step 2: Replace the inline wake setup with a reusable closure**

Replace the current block (lines ~363-371):

```go
				pakDir := strings.TrimSpace(os.Getenv("BMO_PAK_DIR"))
				wakeCfg := wakeAssets{
					ORTLib:    filepath.Join(pakDir, "lib", platform, "libonnxruntime.so"),
					MelModel:  filepath.Join(pakDir, "assets", "wakeword", "melspectrogram.onnx"),
					EmbModel:  filepath.Join(pakDir, "assets", "wakeword", "embedding_model.onnx"),
					WakeModel: filepath.Join(pakDir, "assets", "wakeword", "hey_bmo.onnx"),
				}
				gov := &power.Governor{Logf: logger.Warnf}
				stopWake = startWakeWord(ctx, logger, machine, cfg, audioRouter, audioPipeline, gov, wakeCfg, audioCfg.SampleRate, audioCfg.Channels)
```

with:

```go
				pakDir := strings.TrimSpace(os.Getenv("BMO_PAK_DIR"))
				tmpDir := filepath.Join(os.TempDir(), "BMO")
				_ = os.MkdirAll(tmpDir, 0o755)
				gov := &power.Governor{Logf: logger.Warnf}
				// restartWake (re)builds the wake detector for a mod, resolving its
				// optional custom wake model. Called at startup and on every mod
				// switch (synchronously, on the main goroutine) so the wake word
				// changes with the mod, like the face does.
				restartWake = func(m mod.Mod) {
					if stopWake != nil {
						stopWake() // cancels the loop and Close()s the old detector
					}
					if wakeCleanup != nil {
						wakeCleanup() // remove the previous mod's extracted temp model
					}
					var assets wakeAssets
					assets, wakeCleanup = buildWakeAssets(m, pakDir, platform, tmpDir, logger)
					stopWake = startWakeWord(ctx, logger, machine, cfg, audioRouter, audioPipeline, gov, assets, audioCfg.SampleRate, audioCfg.Channels)
				}
				restartWake(activeMod)
```

(The `defer stopWake()` at lines ~374-379 is unchanged and still closes the final detector on exit. The final mod's extracted temp file is intentionally left for overwrite/`/tmp` cleanup — harmless, per the spec.)

- [ ] **Step 3: Call `restartWake` from the mod-switch handler**

In `reloadMod` (closure starting line 419), after the clip-library/`clipPlayer` rebuild block (around line 458, immediately before the function's closing logic for prompts/scheduler — place it after the audio rebuild so the new mod's audio is ready), add:

```go
			// Rebuild the wake detector for the new mod (picks up its custom wake
			// model, or falls back to stock). Synchronous, on the main goroutine;
			// nil unless AI + audio were initialized at startup.
			if restartWake != nil {
				restartWake(active)
			}
```

> Read `reloadMod` to find the right insertion point: after `clipPlayer = clips.NewPlayer(...)` (the audio rebuild) and before the scheduler/`return`. The exact line shifts as you read; anchor on the `clipPlayer` reassignment, not a hard line number.

- [ ] **Step 4: Build**

Run: `gofmt -w cmd/bmo-pak/main.go cmd/bmo-pak/modwake.go && CGO_ENABLED=1 go build ./...`
Expected: success. (If `power`/`os`/`filepath`/`mod` imports are now unused or newly needed, fix the import block — `mod` and `power` are already imported in main.go.)

- [ ] **Step 5: Full tests + lint**

Run: `CGO_ENABLED=1 go test ./cmd/bmo-pak/ ./internal/... 2>&1 | grep -vE 'Cannot process svg element' && golangci-lint run ./...`
Expected: `ok` for all packages; no lint findings.

- [ ] **Step 6: Commit**

```bash
git add cmd/bmo-pak/main.go
git commit -m "feat(bmo-pak): rebuild wake detector on mod switch (live custom wake word)"
```
No `Co-Authored-By` trailer.

---

## Task 4: Document the convention

**Files:**
- Modify: `docs/MODDING.md`, `README.md`, `training/wakeword/README.md`

- [ ] **Step 1: MODDING.md — add a wake-word section**

In `docs/MODDING.md`, add a section (match the document's existing heading style and the format of the faces/audio sections):

```markdown
## Custom wake word

A mod may ship its own hands-free wake phrase by placing a trained classifier at:

```
<mod>/wakeword/wake.onnx
```

When the mod is selected (and the wake word is enabled in Settings), this model
replaces the stock **"Hey BMO"** trigger immediately — no relaunch needed. Switch
back to a mod without one and the stock model returns.

Requirements:

- The model must satisfy BMO's classifier contract: a single float32 input
  shaped `[_, 16, 96]` and a single float32 output `[_, 1]` (an openWakeWord
  classifier). Only the classifier is overridable; BMO supplies the shared
  melspectrogram and embedding base models.
- If the file is missing or fails the contract, BMO logs a warning and falls
  back to "Hey BMO" — a bad model never breaks the pak.

Train one with the pipeline in [`training/wakeword/`](../training/wakeword/README.md);
its output drops straight into `wakeword/wake.onnx`.
```

- [ ] **Step 2: README.md — note it in the wake-word bullets**

In `README.md`, under **Configuration → Wake word (hands-free)**, after the bullet describing the shipped "Hey BMO" model and `training/wakeword/`, add:

```markdown
- A **mod** can ship its own wake phrase as `wakeword/wake.onnx`; it replaces
  "Hey BMO" while that mod is active (see the [Modding guide](docs/MODDING.md)).
```

- [ ] **Step 3: training/wakeword/README.md — mention the mod drop-in**

In `training/wakeword/README.md`, add a line near where it describes the output model:

```markdown
The trained model is a drop-in for either the pak (`assets/wakeword/hey_bmo.onnx`)
or a mod (`<mod>/wakeword/wake.onnx`, applied when that mod is selected).
```

- [ ] **Step 4: Verify**

Run: `grep -n "wakeword/wake.onnx" docs/MODDING.md README.md training/wakeword/README.md`
Expected: the convention path appears in all three files.

- [ ] **Step 5: Commit**

```bash
git add docs/MODDING.md README.md training/wakeword/README.md
git commit -m "docs: document mod-supplied wake word (wakeword/wake.onnx)"
```
No `Co-Authored-By` trailer.

---

## Final verification (after all tasks)

- [ ] **Full suite + lint**

Run: `CGO_ENABLED=1 go test ./... 2>&1 | grep -vE '\[no test files\]|^ok|Cannot process svg element'` (expect empty) and `golangci-lint run ./...` (expect 0 issues) and `CGO_ENABLED=1 go build ./...` (expect success).

- [ ] **On-device manual check**

Build (`./scripts/release.sh`) and deploy (`./scripts/deploy.sh`). With a mod that contains `wakeword/wake.onnx`, confirm in `BMO.txt` that selecting it logs `wake model: model=mod(<id>)` and the custom phrase wakes BMO; switch back to a mod without one and confirm `model=default` and "Hey BMO" works. Confirm an invalid `wake.onnx` logs the fallback warning and still wakes on "Hey BMO".

---

## Notes for the implementer

- **No co-author trailer** in commits.
- `startWakeWord` is intentionally left unchanged — the feature is additive (`modwake.go` + `main.go` wiring).
- The ORT happy-path (a valid custom model loading on a real device) is not unit-tested because the runtime library is aarch64; the dev-box tests cover resolution + every fallback branch, and `ValidateClassifier`'s accept/reject is already covered by `internal/wakeword/contract_test.go`. The happy path is confirmed on-device.
- Mod IDs come from directory basenames (`mods/<id>`), so they are safe in the `wake-<id>.onnx` temp filename; no sanitization needed.
