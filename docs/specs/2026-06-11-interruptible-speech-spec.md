# Interruptible Speech Specification

**Date:** 2026-06-11
**Status:** Approved
**Scope:** Interrupting BMO's TTS playback and talking animation with the A or B button

## 1. Purpose

While BMO is speaking, the user must be able to cut the speech short:

- **B** stops the sound and the talking animation; BMO returns to idle.
- **A (held)** stops the sound and the talking animation, then starts a normal
  push-to-talk listening session, exactly as if A had been pressed from idle.

## 2. Behavior Requirements

1. Interrupting stops new audio within one pacing chunk (~20ms). The already
   buffered ALSA tail (≤ `audio.PlaybackBufferMs`, 200ms) may finish playing;
   this is accepted.
2. The talking animation and `speaking` state end together with the interrupt
   — never an error/concerned face.
3. An interrupt counts as a normal end of speech: the interaction is recorded
   and the state machine transitions `speaking → idle` via `EventRest`.
4. **B** while speaking only interrupts — it must not exit to NextUI. A second
   B press (now idle, no overlay) exits as usual. B with the settings overlay
   open keeps closing the overlay.
5. **A** while speaking transitions seamlessly into listening: speech is
   interrupted first, then the regular PTT press path runs (`EventListen`,
   capture buffer begins).
6. A/B during listening or thinking are unchanged (out of scope).

## 3. Design

### 3.1 Pipeline interrupt primitive (`internal/assistant/voice.go`)

- `ProcessBatch` wraps the speech playback in a child context registered under
  a mutex (`playCancel`, `playDone`).
- `InterruptSpeech() bool`:
  - returns `false` immediately when no speech is playing;
  - otherwise cancels the playback context and **blocks on `playDone` until
    the pipeline has finished its post-speech state transitions**, then
    returns `true`. Callers never manage assistant state themselves.
- Inside `ProcessBatch`, a `context.Canceled` from the paced player while the
  parent context is still alive is treated as a normal end of speech
  (`RecordInteraction` + `EventRest`), not a failure. Parent-context
  cancellation (shutdown) keeps the existing failure path.
- `EventRest` stays owned by `ProcessBatch`, so an interrupt racing the
  natural end of speech can never double-fire `EventRest` (which from idle
  would put BMO to sleep).

### 3.2 A button (`cmd/bmo-pak/ptt_shared.go`)

In the PTT watcher `ev.Held` branch, call `pipeline.InterruptSpeech()` before
the existing wake/`EventListen`/`buffer.Begin()` sequence. Because the call
blocks until the machine is back in idle, the `EventListen` transition (valid
only from idle) then behaves exactly like a normal PTT press.

### 3.3 B button (`cmd/bmo-pak/main_fb.go`, SDL equivalent if applicable)

In the `NavCancel` branch of `handleNav`: when no overlay is open and
`InterruptSpeech()` returns `true`, consume the press instead of exiting.

## 4. Test Expectations

- Interrupting mid-playback returns from `ProcessBatch` promptly, leaves the
  machine in `idle` (not `error`), and resets the mouth amplitude to 0.
- `InterruptSpeech()` with no active speech returns `false` without blocking.
- The existing playback-clock test continues to guard animation/audio sync.

## 5. Alternatives Considered

- **Kill/restart `aplay` per interrupt** — silences even the 200ms tail but
  requires reworking the audio session lifecycle. Escape hatch if the tail
  ever feels laggy.
- **Cancelling the whole `ProcessBatch` context** — rejected: aborts STT/chat
  mid-flight with `EventFail` (concerned face) and tangles interrupt with
  shutdown semantics.
