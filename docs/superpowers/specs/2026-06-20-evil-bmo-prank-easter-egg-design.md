# Evil BMO Prank Easter Egg — Design

**Date:** 2026-06-20
**Status:** Approved design, ready for implementation planning
**Scope:** A deliberately-hidden, non-mod easter egg in the `bmo-pak` binary. NOT an official mod feature.

## Summary

When AI mode is on **and** the active mod is Evil BMO, Evil BMO will — rarely and
spontaneously, or on demand via a hidden button — try to provoke a *nearby* BMO
device into a conversation. It does this acoustically: it speaks the wake phrase
"Hey BMO" followed by an in-character taunt out of its own speaker, hoping a
nearby device's wake detector trips. It then listens for a reply and either fires
back a contextual comeback (bounded to a few rounds) or, if no one answers, makes
a smug prankster remark about being ignored.

This is treated as an **exception** to the normal mod feature path: it is hardcoded
in the binary, gated on the Evil BMO mod, invisible (no settings, no manifest field,
no docs), and **does not touch** the `examples/mods/evil-bmo/` example directory or
the mod/config systems.

## Gating

The easter egg only exists when **both** hold, re-checked at trigger time (not only
at startup, so toggling AI off or switching mods disables it immediately):

- `cfg.UsesAI()` is true, and
- `activeMod.ID == "evil-bmo"` (the example mod's directory/zip name, hardcoded as a
  constant in the easter-egg file).

## Placement

- **New:** `cmd/bmo-pak/evil_prank.go` + `cmd/bmo-pak/evil_prank_test.go` — gate
  constant, auto-trigger scheduler, the prank sequence, the LLM nudge strings, and
  the pure logic. A header comment marks it explicitly as a deliberately-hidden,
  non-mod easter egg that bypasses the normal mod feature path.
- **Modified (minimal):** `cmd/bmo-pak/main.go` — ~4 lines to compute the gate, start
  the scheduler, and add a D-pad Down branch.
- **Likely modified:** `internal/assistant` — add a *generate-only* counterpart to
  `SpeakRemark` (produce remark text without speaking it) so the taunt text can be
  prepared before the wake word is spoken. The wake loop also gains a check of a
  shared "prank active" flag so it stands down during a prank.
- **Untouched:** `examples/mods/evil-bmo/`, `internal/mod`, `internal/config`,
  `internal/examplemods`, the settings menu, and MODDING docs.

## Triggers

- **Manual (testing / on-demand):** D-pad Down (`input.NavDown`). Today this is a
  no-op outside the settings menu, so repurposing it for the idle case regresses
  nothing. Only starts from `StateIdle`, no menu open, not shutting down, gate true.
- **Automatic (spontaneous):** a dedicated jittered timer with a mean of ~2–4 hours
  of idle time. Heavily randomized so it reads as a surprise rather than a nuisance.
  Resets on any real interaction so it only fires after genuine quiet. Same idle/gate
  preconditions as the manual trigger.

## The Prank Sequence

`runEvilPrank(ctx)` runs the whole bit in its own goroutine, guarded by a single-flight
"prank active" flag (a second trigger mid-prank is ignored). The flag is cleared in a
`defer` even on error/cancel.

At the start, pick `maxRounds = random(2 or 3)`.

0. **Pre-generate the taunt (text only).** An LLM call in Evil BMO persona returns a
   short trick question or cutting barb aimed at another BMO — *no audio yet*. All the
   "thinking" latency is spent here, before the wake word, so there is no tell-tale
   pause between "Hey BMO" and the taunt.
1. **Fused wake + taunt.** Build one string — `"Hey BMO... <taunt>"` (wake phrase may
   randomly be "Hey BEEMO") — and speak it with a **single** `SpeakVerbatim`. Fusing
   into one TTS utterance makes the pause part of the synthesized audio (via the
   ellipsis), eliminating inter-clip gap/scheduler jitter. "Hey BMO" sits clearly at
   the front to trip the other device's detector. The wake phrase is issued **once**,
   round 1 only.
2. **Listen ~20s.** Take a `CaptureRouter.Subscribe()` channel and run one
   end-of-silence capture with a hard 20s deadline — the "long" continued-conversation
   window, **ignoring** the user's `continued_conversation` setting.
3. **React / loop:**
   - **No reply at all (round 1 silence)** → smug "guess nobody's home / sure, ignore
     me" line via `SpeakRemark`. **Done** (no loop — nothing to converse with).
   - **Reply heard** → STT-transcribe the captured PCM, then:
     - **Rounds 2…maxRounds−1:** contextual comeback via `SpeakRemark` (transcript
       embedded in the nudge; the LLM may laugh — Evil BMO persona has a `laugh`
       emotion — or fire back). Listen 20s again.
       - reply heard → next round
       - **silence mid-stream** → short dismissive "lost interest" closer. **Done.**
     - **Final round (round == maxRounds):** instead of an open comeback, generate a
       **conversation-ending** line — dismissive, **no question** — explicitly nudged
       to give the other BMO no hook to continue (a sign-off, not a prompt). **Done.**
4. Release the flag; the machine settles back to idle.

The loop always terminates: at the cap with a deliberate closer, on the first silence,
or immediately if no one ever answered. Worst-case cost per prank is bounded to ~3
volleys (≤3 × [STT + chat + TTS]).

## Edge Cases & Safety

- **Idle-only:** both triggers start only from `StateIdle`, no menu open, not shutting
  down.
- **Single-flight:** the atomic "prank active" flag prevents overlapping pranks.
- **Wake-loop suppression:** while the flag is set, the real wake loop checks it and
  stands down, so it neither grabs the overheard reply nor self-triggers. (Self-trigger
  on Evil BMO's own "Hey BMO" is additionally prevented by the wake controller already
  suppressing detection while the machine is in the Speaking state.)
- **Interruptible:** B (`NavCancel`) interrupts speech/listening and aborts the prank
  like any other activity; shutdown (MENU / SIGTERM) cancels via context and clears the
  flag.
- **Runtime gate changes:** AI toggled off or mod switched disables it immediately
  (gate re-checked at trigger time).
- **Provider failure:** any STT/chat/TTS error aborts the prank quietly (log + return
  to idle), reusing the pipeline's existing error handling — no fallback clips, no
  error face.

## Testing

- **Pure logic, unit-tested** (mirroring the already-split `wakeController`):
  round-count selection (always 2–3), branch selection given reply-present / silence /
  silence-mid-stream, the single-flight guard, and the gate predicate
  (`UsesAI && ID == "evil-bmo"`). Pipeline interactions (`SpeakVerbatim` /
  generate-only / `SpeakRemark` / capture / STT) go through a small interface with a
  fake, so tests assert the *sequence of calls* and the nudges without real
  audio/network.
- **No device-hardware test** for the acoustic trip ("other BMO hears it"); that is
  manual verification on two physical devices.
- **Gates** (per CLAUDE.md): `CGO_ENABLED=1 go test ./...`,
  `golangci-lint run ./...`, and `CGO_ENABLED=1 go build ./...` all clean.

## Open Implementation Details (for the plan)

- Exact shape of the generate-only remark method in `internal/assistant` (new method
  vs. refactor of `SpeakRemark` to share a text-generation step).
- Exact mechanism of the one-shot "capture an utterance within a deadline" helper —
  whether to factor a reusable piece out of the existing wake-capture batching or
  write a small dedicated listener over a `CaptureRouter.Subscribe()` channel.
- Precise auto-cadence numbers and jitter distribution.
- The exact nudge wording for taunt / comeback / closer / no-reply lines.
