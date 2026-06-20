# Mod-supplied Wake Word — Design

**Date:** 2026-06-20
**Status:** Approved design, pending spec review
**Scope:** Let a mod ship its own "Hey BMO" replacement wake-word classifier, applied live on mod switch. Single implementation plan.

## Problem

Mods can override BMO's faces, persona, voice, idle quotes, and emotion vocabulary, but **not** the wake phrase. The wake classifier is loaded from a fixed pak path (`cmd/bmo-pak/main.go` `run()`, `wakeAssets.WakeModel = pakDir/assets/wakeword/hey_bmo.onnx`) that never consults the active mod's filesystem. A modder who trains a custom phrase (the `training/wakeword/` pipeline already produces a drop-in `[1,16,96]→[1,1]` classifier) has no supported way to ship it; they can only manually overwrite the pak's single shared model.

## Verified code facts (serena + cross-check)

- `wakeword.New` (the detector constructor) is referenced in exactly **one** place: `startWakeWord` (`cmd/bmo-pak/wakeword.go:145`). The live swap drives that single site.
- `startWakeWord` and the `wakeAssets` struct have no callers outside `run()` in `main.go`; the whole wake wiring (asset construction at `main.go:364`, the `startWakeWord` call at `main.go:371`, the `stopWake` stop-func, and the mod-switch handler at `main.go:~420`) is local to `run()`.
- `wakeword.ValidateClassifier(path)` + `wakeword.InitEnv(libPath)` are real, tested APIs (consumed by `cmd/wakeword-eval/eval.go:196-199`, covered by `internal/wakeword/contract_test.go`). They enforce the `[1,16,96]→[1,1]` classifier contract. (serena under-reported these — they live behind the `cmd/wakeword-eval` target — so confirmed by grep.)
- The detector is bound **once** at startup; the mod-switch handler currently rebuilds faces/persona/voice/audio but **not** the wake detector.
- `mod.Mod.FS fs.FS` is rooted at the mod's contents (dir or `.zip`); the switch handler already calls `active.Open(...)` before reading `active.FS`. `CaptureRouter.Subscribe()` returns `(<-chan []byte, cancel func())` and supports resubscribe.
- A temp dir is available: `os.TempDir()` / the existing `os.TempDir()/BMO` pattern (`main.go:1181`).

## Decisions (from brainstorming)

- **Convention path** declaration (not a manifest key): a mod ships `wakeword/wake.onnx`.
- **Live swap on mod switch** (not relaunch-only): switching mods rebuilds the detector so the wake word changes immediately, like the face.
- Baked-in defaults: uniform **extract-to-temp** even for directory mods (one code path for dir + zip); **async** detector rebuild on switch; the example `evil-bmo` mod ships **no** real model (a test-fixture mod exercises the path; the capability is documented).

## Design

### Convention
A mod may ship **`wakeword/wake.onnx`** at the root of its FS. Present → it replaces "Hey BMO" while the mod is active; absent → the pak's stock `hey_bmo.onnx`. Only the **classifier** is overridable — the base `melspectrogram.onnx` / `embedding_model.onnx` always come from the pak (phrase-independent, version-stable; matches `training/wakeword/`).

### Resolution + extraction (pure, unit-testable)
New helper (in `cmd/bmo-pak`, near the wake wiring):

```go
// resolveWakeModel returns a filesystem path to the wake classifier to use:
// the active mod's wakeword/wake.onnx (extracted to tmpDir) if present, else
// defaultPath. custom reports whether a mod model was used. Extraction makes
// the model usable by onnxruntime regardless of whether the mod is a dir or zip.
func resolveWakeModel(modFS fs.FS, modID, defaultPath, tmpDir string) (path string, custom bool, err error)
```

- If `wakeword/wake.onnx` is readable from `modFS`, copy its bytes to `tmpDir/wake-<modID>.onnx` (overwrite), return that path + `custom=true`.
- Else return `defaultPath` + `custom=false`.
- onnxruntime needs a real OS path and a mod may be a zip, so we **always extract to a temp file** — one uniform path for both. The temp file lives for the detector's lifetime; it is removed when the detector is stopped/replaced (see stop func below).
- Pure file/FS logic → testable without the ONNX runtime.

### Validation + graceful fallback (device path; ORT-gated)
Before a custom model is used, validate it against the contract with `wakeword.ValidateClassifier`. On any failure (extraction error, validation error, missing ORT) → **log a warning and use the pak default**. Never crash; never load a wrong-shaped model. Validation needs the ORT lib loaded, so this step runs on-device / in env-gated tests (the `cmd/wakeword-eval` tests are the precedent for env-gating ORT on the x86 dev box).

Resolution+validation are composed into a single builder:

```go
// buildWakeAssets fills mel/emb from the pak and resolves WakeModel from the
// active mod (validated), falling back to the pak default on any problem.
func buildWakeAssets(activeMod mod.Mod, pakDir, platform, tmpDir string, logf func(string, ...any)) wakeAssets
```

### Live swap on mod switch
- **Startup** (`main.go:364`): build `wakeCfg` via `buildWakeAssets(activeMod, ...)` instead of hardcoding `hey_bmo.onnx`.
- **Mod-switch handler** (`main.go` after the face/audio rebuild, ~line 457): when AI mode + the wake feature are active, tear down and rebuild:
  - call the existing `stopWake()` (cancels the loop, `Close()`s the detector, and — newly — removes the old temp model file);
  - `stopWake = startWakeWord(ctx, logger, machine, cfg, audioRouter, audioPipeline, gov, buildWakeAssets(active, ...), sampleRate, channels)`.
  - Do the rebuild in a **goroutine** (building the detector loads three ONNX models, a few hundred ms), consistent with the async face-cache warm / anim prewarm already in the switch handler — so the switch doesn't hitch. Guard against overlap: a rebuild in flight must complete/!cancel cleanly before the next (serialize via the same single-threaded switch handler; the goroutine only does the build+start, and `stopWake` is reassigned on the handler's thread).
- The `WakeEngaged` teardown shipped in the previous fix already clears the listening face when the loop stops (`run`'s `defer l.machine.SetWakeEngaged(false)`), so a mid-conversation mod switch degrades cleanly.

### Stop func / temp cleanup
`startWakeWord`'s returned stop func currently does `cancel(); detector.Close()`. Extend it to also remove the extracted temp model file (only when a custom model was extracted), **after** `detector.Close()` so the ONNX session no longer holds the file. The temp path + "custom" flag are captured in the closure.

### Logging
`wake word ready: model=mod(<id>) continued=<...>` vs `model=default continued=<...>`, so the live classifier is debuggable from `BMO.txt`.

### Documentation
- `docs/MODDING.md`: new section — ship `wakeword/wake.onnx`; must satisfy the `[1,16,96]→[1,1]` contract; link to `training/wakeword/`; note it hot-swaps on mod select and silently falls back to "Hey BMO" if missing/invalid.
- `README.md` mod section + `training/wakeword/README.md`: one line each noting the trained model can drop into a mod, not just the pak.

## Testing

- **Unit (`cmd/bmo-pak`, no ORT):** `resolveWakeModel`
  - mod FS with `wakeword/wake.onnx` → returns `tmpDir/wake-<id>.onnx`, `custom=true`, and the extracted bytes equal the source (use the real `assets/wakeword/hey_bmo.onnx` bytes as the fixture, loaded via a `fstest.MapFS` or a temp dir mod);
  - mod FS without it → returns `defaultPath`, `custom=false`;
  - distinct temp filename per mod ID.
- **Env-gated (ORT present):** a valid custom model passes `buildWakeAssets` and yields the custom path; a deliberately wrong-shaped model (e.g. the mel model) falls back to the default path with a logged warning. Gated like the existing `cmd/wakeword-eval` ORT tests (skip on the x86 dev box).
- **Test-fixture mod:** add a minimal example/test mod under the existing example-mods test area containing `wakeword/wake.onnx` (a copy of `hey_bmo.onnx` is a valid fixture) to prove discovery + resolution end-to-end without ORT.
- **Live-swap wiring in `run()`:** not unit-tested (main's `run()` isn't); verified by `CGO_ENABLED=1 go build ./...` + on-device manual check (switch default ↔ a mod with a custom model, confirm the `model=mod(...)`/`model=default` log line flips and detection still works).

## Out of scope / not doing

- No manifest key and no per-mod wake **phrase string** (the model defines the phrase; BMO never needs the text).
- No overriding the base mel/embedding models.
- No real trained model shipped with `evil-bmo` (would need its own "Hey Evil BMO" training run); the capability is documented and exercised by a test fixture.

## Risks / notes

- **Rebuild races:** the switch handler is the single serialization point; the async build must not leave a dangling subscription if a second switch arrives mid-build. Keep the build→start atomic from the handler's perspective (assign `stopWake` only on the handler thread; if the design proves racy in review, fall back to a synchronous rebuild — correctness over the small hitch).
- **Temp file lifetime:** removed by the stop func after `detector.Close()`; a crash leaves a stale `tmpDir/wake-<id>.onnx`, which is harmless and overwritten next run.
- **Zip mod FS reads** during a switch: the existing one-generation deferred `Close` (the recent zip use-after-close fix) means the previous mod FS stays open long enough; the new model is read from the **new** `active.FS`, which `Open` has already populated.
