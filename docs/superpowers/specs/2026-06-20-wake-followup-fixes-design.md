# Wake-word Follow-up Fixes — Design

**Date:** 2026-06-20
**Status:** Approved design, pending spec review
**Scope:** Two related UX bugs in the hands-free ("Hey BMO") wake-word path, both in
`cmd/bmo-pak/wakeword.go` + the render loop / state machine. One implementation plan.

## Problem

The on-device wake-word path works, but its conversational UX diverges from
push-to-talk (PTT) in two ways:

- **Bug A — idle flicker during a wake session.** After BMO reacts, the face
  flickers `listening → neutral (idle) → listening`, and unrelated idle
  animations / proactive remarks play *during* what should be a single engaged
  interaction. PTT never does this.
- **Bug B — premature end-of-utterance cutoff.** A slow speaker (or anyone
  pausing mid-sentence) gets cut off: the first half is processed through
  STT→Chat→TTS, then the continued-conversation follow-up window captures the
  tail as a *second* utterance. The sentence is split across two turns.

## Root causes (verified by reading current code)

### Bug A
The render loop (`cmd/bmo-pak/main.go:879-920`, `case assistant.StateIdle`) runs
the idle face **scheduler** and fires **proactive remarks** on every frame where
`machine.State() == StateIdle`. A wake session passes *through* `StateIdle`
repeatedly:

- `finishCapture` (`wakeword.go:242`) fires `EventRest` (Listening→Idle) **before**
  `ProcessBatch` even starts.
- after the reply, the machine is `Idle` again in the gap before the follow-up
  `beginCapture`.
- the whole continued-conversation follow-up loop bounces through `Idle` between
  captures.

Each idle moment lets the concurrent render loop pick a random idle face or fire a
proactive nudge — exactly the "other faces and animations" observed. PTT avoids
this because it is a single `Idle→Listen→Think→Speak→Idle` pass with no follow-up
loop.

### Bug B
`continueCapture` (`wakeword.go:227`) accumulates `silenceRun` whenever a 0.01-VAD
batch reads as silent and calls `finishCapture` once `silenceRun >= wakeEndSilence`
(fixed **800 ms**). An 800 ms mid-sentence pause therefore ends the capture early;
`finishCapture` processes the first half and opens the follow-up window, which then
grabs the tail.

## Decisions

- **Bug B fix is configurable** (user decision): expose end-of-turn silence as an
  on-device setting rather than a single hard-coded value. Default raised from the
  current 800 ms to a more forgiving value so mid-sentence pauses don't split a
  sentence.
- **Bug A fix mirrors PTT**: suppress the idle scheduler/proactive nudges for the
  entire wake session and hold the listening face, so the session is visually
  identical to PTT.
- **Keep the continued-conversation feature** (follow-up window) as-is — it is
  wanted for genuine second questions. Bug B simply stops it from being
  *mis*-triggered by a split sentence. No state-machine restructure.

## Design

### Bug A — "wake engaged" flag suppresses idle rendering

Introduce a single thread-safe signal shared between the wake goroutine and the
render loop, carried on the existing `assistant.Machine` snapshot (the render loop
already reads one coherent `machine.Snapshot()` per frame, so no new race surface):

- `assistant.Machine` gains:
  - an internal `wakeEngaged bool` guarded by the existing mutex,
  - `SetWakeEngaged(bool)` setter,
  - a `WakeEngaged bool` field on `assistant.Snapshot`, populated in `Snapshot()`.
- The wake loop (`cmd/bmo-pak/wakeword.go`) sets it:
  - **true** at the first wake detection — in `beginCapture` when the session
    starts (transition into the engaged session),
  - **false** in `resetFollowUps()` (conversation truly over) and on wake-loop exit
    (the deferred cleanup in `startWakeWord`'s returned stop func / `run` teardown).
- The render loop's `case assistant.StateIdle` branch (`main.go`):
  - **if `snap.WakeEngaged`**: set `expr = string(assistant.ExpressionListening)` and
    **skip** both `scheduler.Next(...)` and the `proactive.Due(now)` block.
  - else: unchanged (current idle scheduling / gallery / proactive behaviour).

Result: from wake through the end of the follow-up window the listening face is
held steady; `Thinking`/`Speaking` states still drive their own faces normally
(those are non-idle, so the engaged branch is not consulted).

**Lifecycle note:** `SetWakeEngaged(true)` must be idempotent (set on every
`beginCapture`, including follow-up captures) so the flag stays true across the
whole session; it is cleared exactly once when the session ends
(`resetFollowUps`) or the loop tears down.

### Bug B — configurable end-of-turn silence

Mirror the existing `ContinuedConversation` enum pattern exactly.

- `internal/config/config.go`:
  - New constants:
    - `WakeEndSilenceSnappy   = "snappy"`   → ~1.0 s
    - `WakeEndSilenceBalanced = "balanced"` → ~1.3 s (**default**)
    - `WakeEndSilencePatient  = "patient"`  → ~1.6 s
  - New `Config` field `WakeEndSilence string \`json:"wake_end_silence,omitempty"\``.
  - `Default()` sets `WakeEndSilence: WakeEndSilenceBalanced`.
  - `Normalize()`/`Load` validates the value against the three constants and falls
    back to `WakeEndSilenceBalanced` for empty/unknown (same shape as the
    `ContinuedConversation` switch).
- `cmd/bmo-pak/wakeword.go`:
  - Add helper `wakeEndSilenceFor(mode string) time.Duration` (sibling of
    `continuedWindowFor`), mapping the three tiers to durations; unknown →
    balanced.
  - Resolve the duration once when the wake loop is constructed and store it as a
    `wakeLoop` field (e.g. `endSilence time.Duration`), replacing the
    `wakeEndSilence` const usage in `continueCapture`. (The `wakeEndSilence` const
    is removed or repurposed as the balanced default value.)
- `internal/ui/settings_menu.go`:
  - New overlay row `wake_end_silence` labelled e.g. `LISTEN PATIENCE: <VALUE>`,
    `Hidden: !isAI || !m.cfg.WakeWordEnabled`, `Indent: true` — same visibility
    rule as the `continued_convo` row (`settings_menu.go:182`).
  - Cycle handler (mirroring `settings_menu.go:273-274`) advances
    `WakeEndSilence` through `[snappy, balanced, patient]` via `nextInOrder`, and
    persists like the other settings.

## Testing

### Bug B (unit-testable)
- `internal/config`: table test that `Default()` yields `balanced`, and
  `Normalize()`/`Load` maps empty/unknown → `balanced` and preserves the three
  valid values.
- `cmd/bmo-pak`: test `wakeEndSilenceFor` mapping (snappy/balanced/patient/unknown
  → expected durations).
- `cmd/bmo-pak`: a `continueCapture` test (threshold is now an injectable
  `wakeLoop` field) driving silent batches — assert a silence run **below** the
  configured threshold keeps `capturing == true`, and a run **at/above** it calls
  `finishCapture` (transitions out of capture). Reuses the seams in the existing
  `cmd/bmo-pak/wakeword_test.go`.

### Bug A (unit-testable plumbing + reviewed render branch)
- `internal/assistant`: test `SetWakeEngaged(true/false)` is reflected in
  `Snapshot().WakeEngaged`.
- `cmd/bmo-pak`: test the wake-loop lifecycle — `beginCapture` sets
  `WakeEngaged == true`; `resetFollowUps` clears it to `false`; it remains true
  across a follow-up `beginCapture`.
- The render-loop branch itself stays a minimal, reviewed conditional, consistent
  with `main.go`'s render loop being integration-tested rather than unit-tested
  today. (No new test harness for the render loop.)

## Out of scope / not doing

- No change to the follow-up window durations or the continued-conversation
  feature itself.
- No state-machine restructure — the engaged flag is purely additive.
- No numeric/free-form entry for end-of-turn silence — three named tiers fit the
  controller-driven settings menu (no on-screen keyboard needed).

## Risks / notes

- The "engaged" flag must be cleared on **every** exit path of a wake session
  (normal end, near-silent drop, pipeline error, loop teardown) or BMO would stay
  stuck on the listening face after a conversation. `resetFollowUps` is the single
  funnel for "conversation over"; loop teardown is the backstop.
- Suppressing proactive remarks while engaged is intended (they were a source of
  the stray animation), and they resume automatically on the next true return to
  idle.
- Watch device memory when testing on-device: holding the listening face is
  *less* animation than the buggy idle churn, so this should reduce, not increase,
  rasterization load (cf. the prior idle-full-set OOM incident).
