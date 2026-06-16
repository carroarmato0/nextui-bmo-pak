# Active-Face Debug Logging Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Emit a single `debug`-level log line showing exactly which face/animation BMO is rendering and where it came from — `face: rendering "happy" (embedded-default, animated)` — for both embedded-default and mod-override faces, ONLY when the rendered expression changes (no per-frame spam at ~60fps).

**Architecture:** The `internal/face` package gains a pure accessor that reports an expression's source (`"mod-override"` vs `"embedded-default"`); it does NO logging. A tiny, unit-testable change-detector in `package main` (`cmd/bmo-pak/face_log.go`) holds the last-logged expression and returns a formatted line only on change. The render loop in `cmd/bmo-pak/main.go` calls the accessor + `animEngine.Has(expr)` right after `expr` is finalized (after `machine.SetExpression`, current line ~652), feeds the facts to the detector, and emits via `logger.Debugf` only when the detector signals a change. The face package never imports the logger; main owns all logging.

**Tech Stack:** Go, SDL2 (CGO) for the renderer/main build.

---

## Background facts (verified against source — do not re-derive)

- `internal/face/library.go`:
  - `func (l *Library) Bytes(expr string) ([]byte, bool)` — second return is `fromDisk` (`true` = on-disk override won; `false` = embedded default). This is where override-vs-embedded knowledge lives.
  - `func (l *Library) Resolve(expr string) string` — returns the cache key / load name.
  - Self-contained mods (`NewLibraryMode(dir, true)`) never fall back to embedded: a present file is `fromDisk=true`; a folded-to-mod-neutral file is also served from disk (`fromDisk=true`); a truly missing expr returns `(nil, false)`.
- `internal/face/cache.go`: `Cache` wraps a `*Library` (`c.lib`), has the `resolved map[string]string` (expr→key) populated via `c.lib.Resolve`. `renderLocked`/`warmFrame` already call `c.lib.Bytes(name)` and use the `fromDisk` bool only for fallback, not for reporting.
- `internal/face/anim_engine.go`:
  - `func (e *Engine) Has(expr string) bool` — a declared animation def exists (stable). **Chosen for the `animated` label.**
  - `func (e *Engine) Ready(expr string) bool` — def is built AND resident at the current size (timing-dependent; flips as background builds complete). **NOT used for the label**, because it would flip a face between `static` and `animated` across frames as `Prewarm`/`ensureLocked` finish, producing misleading and duplicate-looking log churn. The spec mentions both `Has`/`Ready`; we use `Has` so the label reflects capability and stays stable for a given expression.
- `cmd/bmo-pak/main.go` render loop:
  - `expr` is finalized at line ~652: `machine.SetExpression(assistant.Expression(expr))`. All overrides (clip player, quota, menu) are applied before this point. The log emission goes immediately after this line, before `frame := renderer.FrameState{...}`.
  - Real identifiers in scope: `faceCache` (`*face.Cache`, reassigned by `reloadMod`), `animEngine` (`*face.Engine`, reassigned by `reloadMod`), `logger` (`*observability.Logger`), `expr` (string).
  - Loop runs at ~60fps (`sdl.Delay(16)`); change-detection is mandatory.
  - `internal/renderer/bmo.go`'s `exprTracker.epoch` is renderer-side animation-epoch tracking, NOT logging — do not piggyback on it.
- `internal/observability/logger.go`: `func (l *Logger) Debugf(format string, args ...any)`. `Debugf` is a no-op unless the configured level is `LevelDebug`, so the line only appears when debug logging is enabled.

## File Structure

| File | Change | Purpose |
|------|--------|---------|
| `internal/face/library.go` | modify | Add `func (l *Library) Source(expr string) string` returning `"mod-override"` / `"embedded-default"` (and a sentinel for unresolved). Pure; no logging. |
| `internal/face/cache.go` | modify | Add `func (c *Cache) Source(expr string) string` delegating to `c.lib.Source(expr)` so the render loop calls through its existing `faceCache` reference. |
| `internal/face/library_test.go` | modify | Unit tests for `Library.Source`. |
| `internal/face/cache_test.go` | modify | Unit test for `Cache.Source` delegation. |
| `cmd/bmo-pak/face_log.go` | create | `faceLogger` change-detector + line formatter (pure Go, no SDL deps of its own). |
| `cmd/bmo-pak/face_log_test.go` | create | Unit tests for the change-detector. |
| `cmd/bmo-pak/main.go` | modify | Wire `faceLogger` into the render loop after `expr` is finalized. |

---

## Task 1 — Face source accessor (`internal/face`, no logging)

**Files:** `internal/face/library.go`, `internal/face/cache.go`, `internal/face/library_test.go`, `internal/face/cache_test.go`

Design: `Library.Source` calls `l.Bytes(expr)` and maps the returned `fromDisk` bool + nil-data case to a string. Return values:
- `"mod-override"` when `fromDisk == true` (a non-blank `faces/<name>.svg` or, for self-contained, the mod's own neutral fold won).
- `"embedded-default"` when `fromDisk == false` and bytes are non-nil (embedded asset served).
- `"none"` when `Bytes` returns `(nil, false)` (self-contained mod with no matching face and no neutral — renderer draws its plain fallback). This keeps the accessor total and lets the log line stay accurate rather than mislabeling a missing face as embedded.

`Cache.Source` simply delegates: `return c.Source...` via `c.lib.Source(expr)` (no mutex needed; `Library.Source` only stats/reads files, same as `Resolve` which the cache already calls without extra locking concerns for this purpose).

- [ ] Add failing tests to `internal/face/library_test.go` (follow existing `t.TempDir()` + `os.WriteFile(..., 0o644)` fixture conventions):
  - `TestSourceEmbeddedDefault`: `lib := NewLibrary(filepath.Join(t.TempDir(), "missing"))`; assert `lib.Source(ExprNeutral) == "embedded-default"`.
  - `TestSourceModOverride`: write `neutral.svg` into a tmp dir, `lib := NewLibrary(dir)`; assert `lib.Source("neutral") == "mod-override"`.
  - `TestSourceOverrideViaAlias`: write `crying.svg`, look up via alias `"cry"`; assert `"mod-override"` (mirrors `TestLibraryAliasResolution`).
  - `TestSourceSelfContainedFoldsToModNeutral`: self-contained dir with only `neutral.svg`; assert `lib.Source("sad") == "mod-override"` (folded to mod neutral, still from disk).
  - `TestSourceSelfContainedNoFaceIsNone`: self-contained dir with only `happy.svg`, no neutral; assert `lib.Source("sad") == "none"`.
  - `TestSourceBlankOverrideFallsBack`: write a blank `neutral.svg`; assert `lib.Source(ExprNeutral) == "embedded-default"` (blank override ignored, mirrors `TestLibraryBlankFileIgnored`).
- [ ] Run (expect FAIL — method undefined): `CGO_ENABLED=1 go test ./internal/face/ -run TestSource -v`
- [ ] Add `Library.Source` to `internal/face/library.go` (place after `Resolve`):
  ```go
  // Source reports where expr's rendered bytes come from: "mod-override" when an
  // on-disk faces/<name>.svg (or, for a self-contained mod, its own neutral fold)
  // supplies them, "embedded-default" when the built-in asset is used, or "none"
  // when nothing resolves (self-contained mod with no matching face). It performs
  // the same lookup as Bytes and does no logging.
  func (l *Library) Source(expr string) string {
      data, fromDisk := l.Bytes(expr)
      switch {
      case fromDisk:
          return "mod-override"
      case data != nil:
          return "embedded-default"
      default:
          return "none"
      }
  }
  ```
- [ ] Add `Cache.Source` to `internal/face/cache.go` (place after `Frame`):
  ```go
  // Source reports the origin of expr's rendered bytes — "mod-override",
  // "embedded-default", or "none" — by delegating to the backing Library. It
  // does no logging and is safe to call from the render loop on the same
  // goroutine that calls Frame.
  func (c *Cache) Source(expr string) string {
      return c.lib.Source(expr)
  }
  ```
- [ ] Add a delegation test to `internal/face/cache_test.go`:
  - `TestCacheSourceDelegates`: write `grumpy.svg` into a tmp dir, `c := NewCache(NewLibraryMode(dir, true))`; assert `c.Source("grumpy") == "mod-override"` and `c := NewCache(NewLibrary(""))` gives `c.Source(ExprNeutral) == "embedded-default"`.
- [ ] Run (expect PASS): `CGO_ENABLED=1 go test ./internal/face/ -run 'TestSource|TestCacheSource' -v`
- [ ] Lint: `golangci-lint run ./...`
- [ ] Commit: `feat(face): expose face source (mod-override vs embedded-default) accessor`

## Task 2 — Change-detection helper (`package main`, unit-testable)

**Files:** `cmd/bmo-pak/face_log.go`, `cmd/bmo-pak/face_log_test.go`

Design: a tiny struct living in `package main`. It carries no SDL/CGO imports of its own (only `fmt`), but because other files in `package main` link SDL, its test must build with CGO — run with `CGO_ENABLED=1`. The detector returns `(line, ok)`: `ok == true` only when `expr` differs from the last logged expr; it then records the new expr. This makes change-back (A → B → A) re-log.

- [ ] Write failing tests in `cmd/bmo-pak/face_log_test.go` (`package main`):
  - `TestFaceLogNoteFormatsLine`: fresh `faceLogger{}`; `note("happy", "embedded-default", true)` returns `("face: rendering \"happy\" (embedded-default, animated)", true)`.
  - `TestFaceLogNoteStaticLabel`: `note("neutral", "mod-override", false)` returns line ending `(mod-override, static)`, `ok == true`.
  - `TestFaceLogNoteSuppressesRepeat`: after first `note("happy", ...)`, a second `note("happy", ...)` returns `("", false)`.
  - `TestFaceLogNoteRelogsOnChange`: `note("happy",...)` → `note("sad",...)` (`ok`) → `note("sad",...)` (suppressed) → `note("happy",...)` (`ok` again, change-back re-logs).
  - `TestFaceLogNoteFirstEverLogs`: zero-value `faceLogger`'s first `note("neutral", "embedded-default", false)` returns `ok == true` (initial state always logs once).
- [ ] Run (expect FAIL — type undefined): `CGO_ENABLED=1 go test ./cmd/bmo-pak/ -run TestFaceLog -v`
- [ ] Create `cmd/bmo-pak/face_log.go`:
  ```go
  package main

  import "fmt"

  // faceLogger emits the active-face line once whenever the rendered expression
  // changes, suppressing per-frame repeats. It is not safe for concurrent use;
  // call it only from the render goroutine.
  type faceLogger struct{ last string }

  // note records expr and returns a formatted log line plus true when expr
  // differs from the previously noted expression; otherwise it returns ("", false).
  // source is "mod-override" / "embedded-default" / "none"; animated selects the
  // "animated" vs "static" label.
  func (f *faceLogger) note(expr, source string, animated bool) (string, bool) {
      if expr == f.last {
          return "", false
      }
      f.last = expr
      state := "static"
      if animated {
          state = "animated"
      }
      return fmt.Sprintf("face: rendering %q (%s, %s)", expr, source, state), true
  }
  ```
  Note: the zero value's `last == ""` means a first real expr (e.g. `"neutral"`) always logs; an empty `expr` (transient pre-init) is treated as "no change" and suppressed, which is the desired behavior.
- [ ] Run (expect PASS): `CGO_ENABLED=1 go test ./cmd/bmo-pak/ -run TestFaceLog -v`
- [ ] Lint: `golangci-lint run ./...`
- [ ] Commit: `feat(bmo-pak): add faceLogger change-detector for active-face logging`

## Task 3 — Wire into the render loop (`cmd/bmo-pak/main.go`)

**Files:** `cmd/bmo-pak/main.go`

Design: declare one `var flog faceLogger` ABOVE the render loop (near the other per-loop state, before the `for` that drives drawing — sibling to `errorSince`/`expr` declarations). A value (not pointer) is fine since we call pointer methods on the addressable `var`. Inside the loop, immediately after `machine.SetExpression(assistant.Expression(expr))` (line ~652) and before `var overlay ...`, query the live `faceCache`/`animEngine` references (which `reloadMod` reassigns) and emit on change.

- [ ] Add the detector declaration before the render loop. Locate the existing pre-loop state (where `errorSince`/`expr` live) and add:
  ```go
  var flog faceLogger
  ```
- [ ] Insert the emission immediately after `machine.SetExpression(assistant.Expression(expr))`:
  ```go
  if msg, ok := flog.note(expr, faceCache.Source(expr), animEngine.Has(expr)); ok {
      logger.Debugf("%s", msg)
  }
  ```
  - Uses the real identifiers `faceCache` (`*face.Cache`), `animEngine` (`*face.Engine`), `logger` (`*observability.Logger`). Both `faceCache` and `animEngine` are reassigned by `reloadMod` on the same goroutine, so reading them here always sees the active mod's library — a mod switch with the same expr will re-log because... it will NOT (expr unchanged). That is acceptable for WS2's stated goal (log on rendered-expression change); a mod switch typically also drives an expr change. Note this limitation inline is unnecessary; leave a brief code comment: `// log the active face once per change (debug only)`.
- [ ] Build (CGO required for SDL/main): `CGO_ENABLED=1 go build ./...`
- [ ] Verify the full package compiles and existing main tests still pass: `CGO_ENABLED=1 go test ./cmd/bmo-pak/ -v`
- [ ] Manual verification note (device or local with debug level enabled):
  - Set log level to `debug` (config / env as the project wires `ParseLevel`), then build+deploy: `./scripts/release.sh && ./scripts/deploy.sh`.
  - Tail logs: `./scripts/debug-logs.sh` (device log at `/mnt/SDCARD/.userdata/tg5040/logs/BMO.txt`).
  - Trigger state changes (idle → listening → thinking → speaking with an emotion). Expect exactly one `face: rendering "<expr>" (<source>, <state>)` line per distinct rendered expression, with NO per-frame repeats while the expression is held. Confirm a mod-override face logs `(mod-override, ...)` and a built-in logs `(embedded-default, ...)`, and an animated expression (e.g. speaking) logs `animated`.
- [ ] Lint: `golangci-lint run ./...`
- [ ] Commit: `feat(bmo-pak): log active face once per change in render loop`

## Task 4 — Full suite + lint

**Files:** none (verification only)

- [ ] Full test suite (CGO on, renderer/main need SDL): `CGO_ENABLED=1 go test ./...`
- [ ] Race check on the touched packages: `CGO_ENABLED=1 go test -race ./internal/face/ ./cmd/bmo-pak/`
- [ ] Lint everything: `golangci-lint run ./...`
- [ ] Confirm git tree is clean apart from the intended changes; no stray files. Commit only if the prior task commits left anything uncommitted (otherwise nothing to do).

---

## Self-Review

Confirm before declaring done — maps directly to the spec's two WS2 testing bullets:

- [ ] **Source accessor tested.** `internal/face` exposes `Library.Source` / `Cache.Source` returning `"mod-override"` when a `faces/<name>.svg` override (or self-contained neutral fold) is present and `"embedded-default"` otherwise (plus `"none"` for an unresolvable self-contained lookup). Covered by `TestSource*` in `library_test.go` and `TestCacheSourceDelegates` in `cache_test.go`. The face package does NO logging — it only reports facts (spec: "internal/face exposes the source/animated facts (no logging there)").
- [ ] **Change-detection logs once per change.** `faceLogger.note` returns a line only when the expression changes, suppresses per-frame repeats, and re-logs on change-back. Covered by `TestFaceLog*` in `face_log_test.go`. The render loop emits via `logger.Debugf` (debug-level only, no spam at 60fps) using the real `faceCache`/`animEngine`/`logger` identifiers.
- [ ] **`Has` (not `Ready`) drives the `animated` label** — stable per expression; `Ready` is timing-dependent and would flip the label as background builds finish. Rationale recorded in Background facts.
- [ ] No `Co-Authored-By` trailer in any commit message.
- [ ] `CGO_ENABLED=1 go test ./...` and `golangci-lint run ./...` both clean.
