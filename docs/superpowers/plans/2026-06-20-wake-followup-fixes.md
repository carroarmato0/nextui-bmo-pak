# Wake-word Follow-up Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the hands-free "Hey BMO" wake path behave like push-to-talk — hold the listening face through the whole interaction (no idle flicker), and let the user configure how long a silence ends their sentence.

**Architecture:** Bug A adds a `WakeEngaged` flag carried on the `assistant.Machine` snapshot; the wake loop mirrors the wake controller's session state onto it, and the render loop holds the listening face (skipping the idle scheduler + proactive remarks) while engaged. Bug B turns the fixed 800 ms end-of-utterance silence into a three-tier config setting (`snappy`/`balanced`/`patient`) surfaced as a `LISTEN PATIENCE` settings row.

**Tech Stack:** Go 1.25, existing `internal/assistant` state machine, `cmd/bmo-pak` wake loop, `internal/config`, `internal/ui` settings menu. Tests are standard `go test`. Build/test commands need CGO: `CGO_ENABLED=1 go test ./...`.

**Spec:** `docs/superpowers/specs/2026-06-20-wake-followup-fixes-design.md`

---

## File Structure

- `internal/assistant/state.go` — add `WakeEngaged` to `Snapshot`, a `wakeEngaged` field + `SetWakeEngaged` to `Machine`, and populate it in `Snapshot()`.
- `internal/assistant/state_test.go` — test the new flag round-trips through `Snapshot()`.
- `cmd/bmo-pak/wakeword.go` — `wakeController` gains an `engaged` session flag (`startSession`/`Engaged`, cleared in `resetFollowUps`); `wakeLoop` mirrors it to the machine, gains a configurable `endSilence` field, and uses a pure `captureShouldFinish` predicate; add `wakeEndSilenceFor`.
- `cmd/bmo-pak/wakeword_test.go` — tests for the controller session lifecycle, `wakeEndSilenceFor`, and `captureShouldFinish`.
- `cmd/bmo-pak/main.go` — render loop holds the listening face while `WakeEngaged`.
- `internal/config/config.go` — `WakeEndSilence` field, three constants, default, normalize, `WakeEndSilenceLevels()`.
- `internal/config/config_test.go` — default + normalize coverage for the new field.
- `internal/ui/settings_menu.go` — new `wake_end_silence` row, `settingsSlotCount` bump, `ToggleFocused` index shift + new case.
- `internal/ui/settings_menu_test.go` — order list + shifted indices + a cycle test.
- `README.md` — document the `LISTEN PATIENCE` setting.

---

## Task 1: `Machine.WakeEngaged` flag (assistant layer)

**Files:**
- Modify: `internal/assistant/state.go` (struct `Snapshot` ~line 24, struct `Machine` ~line 106, `Snapshot()` ~line 184)
- Test: `internal/assistant/state_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/assistant/state_test.go`:

```go
func TestSetWakeEngagedRoundTripsThroughSnapshot(t *testing.T) {
	m := NewMachine()
	if m.Snapshot().WakeEngaged {
		t.Fatal("new machine should not be wake-engaged")
	}
	m.SetWakeEngaged(true)
	if !m.Snapshot().WakeEngaged {
		t.Fatal("WakeEngaged should be true after SetWakeEngaged(true)")
	}
	m.SetWakeEngaged(false)
	if m.Snapshot().WakeEngaged {
		t.Fatal("WakeEngaged should be false after SetWakeEngaged(false)")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `CGO_ENABLED=1 go test ./internal/assistant/ -run TestSetWakeEngaged -v`
Expected: FAIL — `m.SetWakeEngaged undefined` / `WakeEngaged` not a field.

- [ ] **Step 3: Implement**

In `internal/assistant/state.go`, add to the `Snapshot` struct (after `IdleSeed int64`):

```go
	IdleSeed        int64
	WakeEngaged     bool
```

Add to the `Machine` struct (after `idleSeed int64`):

```go
	idleSeed        int64
	wakeEngaged     bool
```

Add a setter near the other `Set*` methods (e.g. after `SetIdleSeed`):

```go
// SetWakeEngaged marks whether a hands-free wake-word interaction is in progress.
// While engaged, the render loop holds the listening face and suppresses the idle
// scheduler/proactive remarks so the session matches push-to-talk. It is set by
// the wake loop and read via Snapshot by the render loop.
func (m *Machine) SetWakeEngaged(engaged bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.wakeEngaged = engaged
}
```

In `Snapshot()`, add the field to the returned struct literal (after `IdleSeed: m.idleSeed,`):

```go
		IdleSeed:        m.idleSeed,
		WakeEngaged:     m.wakeEngaged,
```

- [ ] **Step 4: Run test to verify it passes**

Run: `CGO_ENABLED=1 go test ./internal/assistant/ -run TestSetWakeEngaged -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/assistant/state.go internal/assistant/state_test.go
git commit -m "feat(assistant): add WakeEngaged flag to Machine snapshot"
```

---

## Task 2: `wakeController` session lifecycle (Bug A logic)

**Files:**
- Modify: `cmd/bmo-pak/wakeword.go` (`wakeController` struct ~line 32, `resetFollowUps` ~line 93)
- Test: `cmd/bmo-pak/wakeword_test.go`

The controller is the testable source of truth for "is a wake session active". A session starts when the loop begins a capture and ends when the conversation returns to idle (`resetFollowUps`). A follow-up capture must keep it active.

- [ ] **Step 1: Write the failing test**

Add to `cmd/bmo-pak/wakeword_test.go` (the existing `newTestController` helper at the top of the file builds a `*wakeController`):

```go
func TestWakeSessionEngagedLifecycle(t *testing.T) {
	c := newTestController()
	if c.Engaged() {
		t.Fatal("fresh controller must not be engaged")
	}
	c.startSession()
	if !c.Engaged() {
		t.Fatal("startSession must engage")
	}
	// A follow-up capture keeps the session engaged.
	c.startFollowUp()
	if !c.Engaged() {
		t.Fatal("follow-up must remain engaged")
	}
	// Conversation over.
	c.resetFollowUps()
	if c.Engaged() {
		t.Fatal("resetFollowUps must disengage")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `CGO_ENABLED=1 go test ./cmd/bmo-pak/ -run TestWakeSessionEngagedLifecycle -v`
Expected: FAIL — `c.Engaged undefined` / `c.startSession undefined`.

- [ ] **Step 3: Implement**

In `cmd/bmo-pak/wakeword.go`, add an `engaged bool` field to the `wakeController` struct (alongside `followUps int`):

```go
	maxFollowUps    int
	followUps       int
	engaged         bool
```

Add the two methods (place near `resetFollowUps`):

```go
// startSession marks the beginning of a wake interaction. Idempotent: it is
// called on the first capture and on every follow-up capture, so the session
// stays engaged for the whole conversation.
func (c *wakeController) startSession() {
	c.engaged = true
}

// Engaged reports whether a wake interaction is in progress (from the first
// capture until the conversation returns to idle).
func (c *wakeController) Engaged() bool {
	return c.engaged
}
```

In `resetFollowUps`, also clear the session flag:

```go
// resetFollowUps is called when the conversation returns to idle.
func (c *wakeController) resetFollowUps() {
	c.followUps = 0
	c.engaged = false
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `CGO_ENABLED=1 go test ./cmd/bmo-pak/ -run TestWakeSessionEngagedLifecycle -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/bmo-pak/wakeword.go cmd/bmo-pak/wakeword_test.go
git commit -m "feat(bmo-pak): track wake-session engaged state in wakeController"
```

---

## Task 3: Wire engaged flag through the loop + render loop (Bug A behaviour)

**Files:**
- Modify: `cmd/bmo-pak/wakeword.go` (`run` ~line 176, `beginCapture` ~line 190, `finishCapture` ~line 242)
- Modify: `cmd/bmo-pak/main.go` (render loop `case assistant.StateIdle:` ~line 880)

This is wiring (mirror the controller's session state onto the machine, and consume it in the render loop). It is verified by build + the existing suites; the render-loop branch is a reviewed conditional (the render loop is not unit-tested today, consistent with the spec).

- [ ] **Step 1: Mirror the flag in `beginCapture`**

In `cmd/bmo-pak/wakeword.go`, `beginCapture` currently starts with `l.machine.Transition(assistant.EventListen)`. Add the session start + mirror as the first lines:

```go
func (l *wakeLoop) beginCapture(now time.Time) {
	l.wc.startSession()
	l.machine.SetWakeEngaged(l.wc.Engaged())
	l.machine.Transition(assistant.EventListen)
	l.buffer.Begin()
	l.capturing = true
	l.captureStart = now
	l.silenceRun = 0
	l.detector.Reset()
}
```

- [ ] **Step 2: Mirror the flag in `finishCapture`**

In `finishCapture`, the conversation-over branch calls `l.wc.resetFollowUps()`. Mirror the cleared flag right after it. The end of `finishCapture` currently looks like:

```go
	if l.wc.windowOpen(time.Now()) {
		l.wc.startFollowUp()
		l.logger.Debugf("continued conversation: follow-up window open")
		l.beginCapture(time.Now())
		return
	}

	l.wc.resetFollowUps()
}
```

Change the final lines to:

```go
	l.wc.resetFollowUps()
	l.machine.SetWakeEngaged(l.wc.Engaged())
}
```

(The follow-up branch returns via `beginCapture`, which re-mirrors `true`, so it stays engaged.)

- [ ] **Step 3: Clear the flag on loop teardown**

In `run`, guarantee the flag is cleared when the loop exits (ctx cancelled or channel closed) so BMO never stays stuck on the listening face:

```go
func (l *wakeLoop) run(ctx context.Context, sub <-chan []byte) {
	defer l.machine.SetWakeEngaged(false)
	for {
		select {
		case <-ctx.Done():
			return
		case batch, ok := <-sub:
			if !ok {
				return
			}
			l.handleBatch(ctx, batch)
		}
	}
}
```

- [ ] **Step 4: Hold the listening face in the render loop**

In `cmd/bmo-pak/main.go`, the `case assistant.StateIdle:` branch begins with `errorSince = time.Time{}` then the gallery/scheduler/proactive logic. Insert the engaged short-circuit immediately after `errorSince = time.Time{}`:

```go
		case assistant.StateIdle:
			errorSince = time.Time{}
			if snap.WakeEngaged {
				// A hands-free wake interaction is in progress: hold the listening
				// face for the whole session, exactly like push-to-talk — no idle
				// scheduler, no proactive remarks mid-conversation. They resume on
				// the next true return to idle.
				expr = string(assistant.ExpressionListening)
				break
			}
			if galleryActive && galleryIdx >= 0 && galleryIdx < len(galleryFaces) {
				// ... existing body unchanged ...
```

(The `break` exits the `switch`; `expr` is consumed by the render code after the switch.)

- [ ] **Step 5: Build and run the wake + assistant suites**

Run: `CGO_ENABLED=1 go build ./... && CGO_ENABLED=1 go test ./cmd/bmo-pak/ ./internal/assistant/ 2>&1 | grep -vE 'Cannot process svg element'`
Expected: build succeeds; both packages report `ok`.

- [ ] **Step 6: Lint**

Run: `golangci-lint run ./cmd/bmo-pak/... ./internal/assistant/...`
Expected: no new findings.

- [ ] **Step 7: Commit**

```bash
git add cmd/bmo-pak/wakeword.go cmd/bmo-pak/main.go
git commit -m "feat(bmo-pak): hold listening face during wake session (no idle flicker)"
```

---

## Task 4: `WakeEndSilence` config setting (Bug B config)

**Files:**
- Modify: `internal/config/config.go` (constants ~line 23, `Config` struct ~line 177, `Default()` ~line 205, `Normalize`/`Load` switch ~line 266)
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/config/config_test.go`:

```go
func TestWakeEndSilenceDefaultAndNormalize(t *testing.T) {
	if got := Default().WakeEndSilence; got != WakeEndSilenceBalanced {
		t.Fatalf("default WakeEndSilence = %q, want %q", got, WakeEndSilenceBalanced)
	}
	cases := map[string]string{
		"":         WakeEndSilenceBalanced, // empty -> default
		"nonsense": WakeEndSilenceBalanced, // unknown -> default
		WakeEndSilenceSnappy:   WakeEndSilenceSnappy,
		WakeEndSilenceBalanced: WakeEndSilenceBalanced,
		WakeEndSilencePatient:  WakeEndSilencePatient,
	}
	for in, want := range cases {
		c := Default()
		c.WakeEndSilence = in
		c.Normalize()
		if c.WakeEndSilence != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, c.WakeEndSilence, want)
		}
	}
}
```

> `Normalize()` is an exported method on `*Config` (`config.go:256`); `Load` calls it, which is how the on-device config gets defaulted. The existing `TestNormalizeProactiveTalk` in `config_test.go` uses the same `cfg.Normalize()` pattern.

- [ ] **Step 2: Run test to verify it fails**

Run: `CGO_ENABLED=1 go test ./internal/config/ -run TestWakeEndSilence -v`
Expected: FAIL — `WakeEndSilenceBalanced` / `WakeEndSilence` undefined.

- [ ] **Step 3: Implement**

In `internal/config/config.go`, add constants next to the `ContinuedConvo*` block:

```go
	// Wake end-of-turn silence tiers: how long a pause must last before BMO
	// treats your sentence as finished. Mapped to durations in cmd/bmo-pak.
	WakeEndSilenceSnappy   = "snappy"   // ~1.0s
	WakeEndSilenceBalanced = "balanced" // ~1.3s (default)
	WakeEndSilencePatient  = "patient"  // ~1.6s
```

Add the field to the `Config` struct (next to `ContinuedConversation`):

```go
	WakeEndSilence        string        `json:"wake_end_silence,omitempty"`
```

In `Default()`, add (next to `ContinuedConversation: ContinuedConvoShort,`):

```go
		WakeEndSilence:        WakeEndSilenceBalanced,
```

In the normalize path (right after the `ContinuedConversation` switch), add:

```go
	switch c.WakeEndSilence {
	case WakeEndSilenceSnappy, WakeEndSilenceBalanced, WakeEndSilencePatient:
	default:
		// Empty or unknown: default to balanced.
		c.WakeEndSilence = WakeEndSilenceBalanced
	}
```

Add an ordered-levels helper (used by the settings menu cycle + tests), next to the other `Supported*`/levels helpers:

```go
// WakeEndSilenceLevels returns the end-of-turn silence tiers in cycle order.
func WakeEndSilenceLevels() []string {
	return []string{WakeEndSilenceSnappy, WakeEndSilenceBalanced, WakeEndSilencePatient}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `CGO_ENABLED=1 go test ./internal/config/ -run TestWakeEndSilence -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): add configurable wake end-of-turn silence (snappy/balanced/patient)"
```

---

## Task 5: `wakeEndSilenceFor` + plumb configurable silence into the loop (Bug B behaviour)

**Files:**
- Modify: `cmd/bmo-pak/wakeword.go` (const `wakeEndSilence` ~line 17, `continueCapture` ~line 227, `wakeLoop` struct ~line 161, `startWakeWord` loop construction ~line 140, helper near `continuedWindowFor` ~line 95)
- Test: `cmd/bmo-pak/wakeword_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `cmd/bmo-pak/wakeword_test.go`:

```go
func TestWakeEndSilenceForMapsTiers(t *testing.T) {
	cases := map[string]time.Duration{
		config.WakeEndSilenceSnappy:   1000 * time.Millisecond,
		config.WakeEndSilenceBalanced: 1300 * time.Millisecond,
		config.WakeEndSilencePatient:  1600 * time.Millisecond,
		"":                            1300 * time.Millisecond, // unknown -> balanced
	}
	for in, want := range cases {
		if got := wakeEndSilenceFor(in); got != want {
			t.Errorf("wakeEndSilenceFor(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestCaptureShouldFinish(t *testing.T) {
	l := &wakeLoop{endSilence: 1300 * time.Millisecond}
	now := time.Unix(100, 0)
	l.captureStart = now

	// Silence below the configured threshold: keep capturing.
	l.silenceRun = 1200 * time.Millisecond
	if l.captureShouldFinish(now) {
		t.Error("should keep capturing below endSilence threshold")
	}
	// Silence at/above the threshold: finish.
	l.silenceRun = 1300 * time.Millisecond
	if !l.captureShouldFinish(now) {
		t.Error("should finish at endSilence threshold")
	}
	// Max-capture cap also finishes regardless of silence.
	l.silenceRun = 0
	if !l.captureShouldFinish(now.Add(wakeMaxCapture)) {
		t.Error("should finish at wakeMaxCapture cap")
	}
}
```

(Ensure `"time"` and the `config` import are present in the test file; the existing tests already import `config`.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `CGO_ENABLED=1 go test ./cmd/bmo-pak/ -run 'TestWakeEndSilenceForMapsTiers|TestCaptureShouldFinish' -v`
Expected: FAIL — `wakeEndSilenceFor` undefined, `endSilence` not a field, `captureShouldFinish` undefined.

- [ ] **Step 3: Implement the helper + predicate + field**

In `cmd/bmo-pak/wakeword.go`:

Remove the standalone `wakeEndSilence` const from the `const (...)` block (lines ~15-21) — it is replaced by the tier helper. Keep `wakeGuardTail`, `wakeMaxCapture`, `wakeMaxFollowUp`, `wakeVADLevel`.

Add the tier→duration helper next to `continuedWindowFor`:

```go
// wakeEndSilenceFor maps a config end-of-turn silence tier to the trailing-
// silence duration that ends a capture. Unknown values map to balanced.
func wakeEndSilenceFor(tier string) time.Duration {
	switch tier {
	case config.WakeEndSilenceSnappy:
		return 1000 * time.Millisecond
	case config.WakeEndSilencePatient:
		return 1600 * time.Millisecond
	default: // balanced / empty / unknown
		return 1300 * time.Millisecond
	}
}
```

Add the `endSilence` field to the `wakeLoop` struct (after `bytesPerSec int`):

```go
	bytesPerSec int
	endSilence  time.Duration
```

Add the pure predicate (place near `continueCapture`):

```go
// captureShouldFinish reports whether the current capture is over: either a
// trailing silence of at least the configured end-of-turn duration, or the hard
// max-capture cap.
func (l *wakeLoop) captureShouldFinish(now time.Time) bool {
	return l.silenceRun >= l.endSilence || now.Sub(l.captureStart) >= wakeMaxCapture
}
```

Rewrite the tail of `continueCapture` to use the predicate. It currently reads:

```go
	if l.silenceRun < wakeEndSilence && now.Sub(l.captureStart) < wakeMaxCapture {
		return
	}
	l.finishCapture(ctx)
```

Replace with:

```go
	if !l.captureShouldFinish(now) {
		return
	}
	l.finishCapture(ctx)
```

In `startWakeWord`, set the field when constructing the loop (after `bytesPerSec: bytesPerSec,`):

```go
		bytesPerSec: bytesPerSec,
		endSilence:  wakeEndSilenceFor(cfg.WakeEndSilence),
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `CGO_ENABLED=1 go test ./cmd/bmo-pak/ -run 'TestWakeEndSilenceForMapsTiers|TestCaptureShouldFinish' -v`
Expected: PASS

- [ ] **Step 5: Full package test + lint**

Run: `CGO_ENABLED=1 go test ./cmd/bmo-pak/ && golangci-lint run ./cmd/bmo-pak/...`
Expected: `ok`; no new lint findings.

- [ ] **Step 6: Commit**

```bash
git add cmd/bmo-pak/wakeword.go cmd/bmo-pak/wakeword_test.go
git commit -m "feat(bmo-pak): use configurable end-of-turn silence in wake capture"
```

---

## Task 6: `LISTEN PATIENCE` settings row (Bug B UI)

**Files:**
- Modify: `internal/ui/settings_menu.go` (`settingsSlotCount` line 142, `slots()` row list ~line 182, `ToggleFocused` ~lines 272-284)
- Test: `internal/ui/settings_menu_test.go` (order list ~line 22, any `focusForTest`/`Items[i]` with index ≥ 17)

> **Critical:** this menu is index-coupled. Inserting a row after `continued_convo` (index 16) shifts `mod` 17→18, `restore_defaults` 19→20, `about` 20→21, and the spacer 18→19. `settingsSlotCount` must go 21→22. Every `ToggleFocused` case and every test index ≥ 17 must move with it.

- [ ] **Step 1: Update the order test (failing)**

In `internal/ui/settings_menu_test.go`, the `want` slice (around line 22) currently has `"wake_word", "continued_convo", "mod",`. Insert the new code:

```go
		"wake_word", "continued_convo", "wake_end_silence", "mod",
```

- [ ] **Step 2: Run to verify it fails**

Run: `CGO_ENABLED=1 go test ./internal/ui/ -run TestSettingsMenu -v 2>&1 | grep -vE 'Cannot process svg element' | head`
Expected: FAIL — order mismatch at index 17 (`mod` where `wake_end_silence` expected), and likely an out-of-range / wrong-code on later index assertions.

- [ ] **Step 3: Insert the row in `slots()`**

In `internal/ui/settings_menu.go`, add the new row immediately after the `continued_convo` row (line 182), mirroring its visibility rule:

```go
		{OverlayItem{Code: "continued_convo", Label: "CONTINUED CONVO: " + strings.ToUpper(m.cfg.ContinuedConversation), Selected: true, Hidden: !isAI || !m.cfg.WakeWordEnabled, Indent: true}, isAI && m.cfg.WakeWordEnabled},
		// LISTEN PATIENCE (end-of-turn silence) only applies with the wake word on.
		{OverlayItem{Code: "wake_end_silence", Label: "LISTEN PATIENCE: " + strings.ToUpper(m.cfg.WakeEndSilence), Selected: true, Hidden: !isAI || !m.cfg.WakeWordEnabled, Indent: true}, isAI && m.cfg.WakeWordEnabled},
		{OverlayItem{Code: "mod", Label: "MOD: " + m.modLabel(), Selected: true}, true},
```

- [ ] **Step 4: Bump `settingsSlotCount`**

Change line 142:

```go
const settingsSlotCount = 22
```

- [ ] **Step 5: Shift `ToggleFocused` cases + add the new case**

In `ToggleFocused`, the indices at/after the insert point move by one. Update:

- `case 16:` (continued_convo) — unchanged.
- Add a new `case 17:` for `wake_end_silence`.
- `case 17:` → `case 18:` (was `cycleMod`).
- `case 19:` → `case 20:` (was restore).
- `case 20:` → `case 21:` (was about).

Concretely, replace the block:

```go
	case 16:
		m.cfg.ContinuedConversation = nextInOrder(m.cfg.ContinuedConversation,
			[]string{config.ContinuedConvoOff, config.ContinuedConvoShort, config.ContinuedConvoLong})
	case 17:
		m.cycleMod()
	case 19:
		if m.onRestore != nil {
			return m.onRestore()
		}
	case 20:
		if m.about != nil {
			m.aboutActive = true
		}
```

with:

```go
	case 16:
		m.cfg.ContinuedConversation = nextInOrder(m.cfg.ContinuedConversation,
			[]string{config.ContinuedConvoOff, config.ContinuedConvoShort, config.ContinuedConvoLong})
	case 17:
		m.cfg.WakeEndSilence = nextInOrder(m.cfg.WakeEndSilence, config.WakeEndSilenceLevels())
	case 18:
		m.cycleMod()
	case 20:
		if m.onRestore != nil {
			return m.onRestore()
		}
	case 21:
		if m.about != nil {
			m.aboutActive = true
		}
```

- [ ] **Step 6: Fix shifted indices in the rest of the test file**

Search the test file for index-dependent assertions that now point one slot too low:

Run: `grep -nE 'focusForTest\(1[789]|focusForTest\(2[01]|Items\[1[789]\]|Items\[2[01]\]|idx 1[789]|idx 2[01]' internal/ui/settings_menu_test.go`

For each hit that targets `mod`/`restore_defaults`/`about`/`spacer` (original indices 17/18/19/20), increment the index by 1 (→ 18/19/20/21) and update any `idx N` message text to match. Assertions for `wake_word` (15), `continued_convo` (16), `request_timeout` (13) and earlier are unchanged.

- [ ] **Step 7: Add a cycle test for the new row**

Add to `internal/ui/settings_menu_test.go`:

```go
func TestSettingsMenuWakeEndSilenceCycles(t *testing.T) {
	cfg := config.Default()
	cfg.Mode = config.ModeAI
	cfg.WakeWordEnabled = true
	m := NewSettingsMenu(cfg)
	m.focusForTest(17)
	if got := m.Overlay().Items[17].Code; got != "wake_end_silence" {
		t.Fatalf("expected wake_end_silence at idx 17, got %q", got)
	}
	// balanced -> patient -> snappy -> balanced (order from WakeEndSilenceLevels)
	start := m.Config().WakeEndSilence
	seen := map[string]bool{start: true}
	for i := 0; i < 3; i++ {
		if err := m.ToggleFocused(); err != nil {
			t.Fatalf("ToggleFocused: %v", err)
		}
		seen[m.Config().WakeEndSilence] = true
	}
	for _, tier := range config.WakeEndSilenceLevels() {
		if !seen[tier] {
			t.Errorf("cycling never visited %q", tier)
		}
	}
}
```

- [ ] **Step 8: Run the settings tests**

Run: `CGO_ENABLED=1 go test ./internal/ui/ 2>&1 | grep -vE 'Cannot process svg element'`
Expected: `ok`

- [ ] **Step 9: Lint**

Run: `golangci-lint run ./internal/ui/...`
Expected: no new findings.

- [ ] **Step 10: Commit**

```bash
git add internal/ui/settings_menu.go internal/ui/settings_menu_test.go
git commit -m "feat(ui): add LISTEN PATIENCE setting row for wake end-of-turn silence"
```

---

## Task 7: Document `LISTEN PATIENCE` in the README

**Files:**
- Modify: `README.md` (Wake word section, the bullet list around lines 163-172)

- [ ] **Step 1: Add the documentation bullet**

In `README.md`, under **Configuration → Wake word (hands-free)**, add a bullet immediately after the existing **CONTINUED CONVO** bullet so the two related settings read as a pair:

```markdown
- **LISTEN PATIENCE** (`wake_end_silence`: `snappy`/`balanced`/`patient`, default
  `balanced`) sets how long a pause must last before BMO treats your sentence as
  finished. `patient` avoids cutting off slow or thoughtful speech; `snappy`
  responds sooner. It is independent of **CONTINUED CONVO** (which controls the
  follow-up window after a reply).
```

- [ ] **Step 2: Verify it renders sensibly**

Run: `grep -n -A4 'LISTEN PATIENCE' README.md`
Expected: the new bullet appears once, well-formed, right after the CONTINUED CONVO bullet.

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs(readme): document LISTEN PATIENCE wake end-of-turn setting"
```

---

## Final verification (after all tasks)

- [ ] **Full suite green**

Run: `CGO_ENABLED=1 go test ./... 2>&1 | grep -vE '\[no test files\]|^ok|Cannot process svg element'`
Expected: empty output (no failures).

- [ ] **Lint clean**

Run: `golangci-lint run ./...`
Expected: no findings.

- [ ] **Build the release artifacts compile** (optional but recommended before on-device test)

Run: `CGO_ENABLED=1 go build ./...`
Expected: success.

On-device manual check (when a device is connected): trigger "Hey BMO", confirm the listening face holds steady through the reply and the follow-up window (no idle faces / proactive remarks mid-conversation), and that `LISTEN PATIENCE: PATIENT` lets a slow sentence complete in one capture.

---

## Notes for the implementer

- **No co-author trailer** in commits.
- The TrimUI button mapping is A=`BTN_EAST` (confirm/PTT), B=`BTN_SOUTH` (cancel) — not relevant to edits here but don't "fix" it if you see it.
- Render-loop code in `main.go` is not unit-tested; Task 3 Step 4 is a reviewed conditional verified by build + manual on-device check, by design.
- Durations live in `cmd/bmo-pak` (`wakeEndSilenceFor`), tier strings + ordering live in `internal/config` — keep that split (config has no time dependency on the loop).
