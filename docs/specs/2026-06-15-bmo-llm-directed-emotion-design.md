# BMO LLM-Directed Facial Emotion — Design Spec

**Date:** 2026-06-15
**Status:** Approved

## Goal

Let the LLM choose BMO's facial emotion. The chat model embeds an emotion
directive in its reply; BMO strips the directive from the text it speaks aloud
and shows the matching face for the duration of the utterance. This makes the 25
expressions added in the previous pass (`2026-06-15-bmo-new-expressions-design.md`)
reachable through normal AI conversation.

## Background

The face layer already renders 33 expressions (`internal/face`, resolved via
`face.Canonical`). The assistant layer (`internal/assistant`) only knows ~11 of
them as `Expression` constants and selects faces purely from the state machine's
current *state* (idle/listening/thinking/speaking/sleeping/error). Nothing lets
the conversation pick an emotion, so the 25 new faces are unreachable in normal
operation.

The render loop (`cmd/bmo-pak/main.go`) maps the current state to an expression
string and passes it to the renderer. The Speaking state is hardcoded to
`ExpressionSpeaking` (the amplitude-driven lip-synced talking mouth).

## Approach

End-to-end LLM-directed emotion, in four parts:

### (a) Vocabulary constants

Add `assistant.Expression` constants for every face expression not already
present (24 new; `laugh` already exists). Define a curated `EmotionVocabulary`
`[]Expression` listing the **conversational** emotions — every expression
**except** the functional, state-driven faces: `listening`, `thinking`,
`speaking`, `sleeping`, `blink`, `look_around`. Also excluded: `whistle` — it
has no face asset and `face.Canonical` folds it to `neutral`, so advertising it
would offer a face BMO cannot actually show (enforced by
`TestEmotionVocabularyResolvesToItself`). The vocabulary drives both the parser
whitelist and the prompt advertising, so they cannot drift apart.

Vocabulary (28): `neutral`, `smile`, `happy`, `laugh`, `content`, `sad`,
`angry`, `surprised`, `excited`, `love`, `shy`, `crying`, `teary`, `gloomy`,
`dizzy`, `unamused`, `annoyed`, `skeptical`, `playful`, `kiss`, `grimace`,
`shout`, `dead`, `glitch`, `dismayed`, `adoring`, `sparkle`, `concerned`.

### (b) Machine directive

- Add an `emotion Expression` field to `Machine`, exposed as
  `Snapshot().Emotion`, with a `SetEmotion(Expression)` setter (mutex-guarded
  like the other setters).
- The directive is **cleared on `EventRest`** (the transition back to idle after
  speech), so an emotion never leaks into the next turn.

### (c) Prompt advertising

A fixed `emotionProtocolPrompt` string (built from `EmotionVocabulary`) is
**appended** to the resolved persona prompt inside
`VoicePipeline.currentSystemPrompt()`. Appending—rather than replacing—keeps any
user-configured persona intact and guarantees the protocol is present for every
chat call (`ProcessBatch` and `SpeakRemark`). `SpeakVerbatim` does not call the
chat model, so curated quotes carry no emotion and are unaffected.

The protocol instructs the model: optionally include exactly one directive of
the form `[emotion]` (one of the listed names); the bracketed word is silent and
never spoken; use it only when it fits the mood.

### (d) Parser + wiring

New file `internal/assistant/emotion.go`:

- `ParseEmotion(reply string) (clean string, emotion Expression)`:
  - Scans for `[word]` tokens; a token matches only if `word`
    (case-insensitive, trimmed) is in `EmotionVocabulary`.
  - Removes every matched token from the text and tidies the resulting
    whitespace (collapse the doubled space left behind, trim ends).
  - Returns the cleaned text and the **first** matched emotion (empty
    `Expression` if none).
  - Non-emotion brackets (e.g. `[pauses]`, `[1]`) pass through untouched.

Wiring in `voice.go`, in both `ProcessBatch` and `SpeakRemark`, after the chat
reply is received and before TTS:

```
clean, emotion := ParseEmotion(reply)
if emotion != "" {
    p.machine.SetEmotion(emotion)
}
// use `clean` as the TTS Input and for onSpoken/logging
```

`speak()` continues to own `EventSpeak`/`EventRest`; the emotion set above is
visible during the Speaking state and cleared when `speak()` issues `EventRest`.

### Render loop

In `cmd/bmo-pak/main.go`, the Speaking case becomes:

```
case assistant.StateSpeaking:
    errorSince = time.Time{}
    if snap.Emotion != "" {
        expr = string(snap.Emotion)
    } else {
        expr = string(assistant.ExpressionSpeaking)
    }
```

No directive → today's lip-synced talking mouth (backwards compatible). A
directive → the static emotion face held for the whole utterance.

## Data flow

```
STT → chat.Reply → reply text
                     │
                     ├─ ParseEmotion → (clean text, emotion)
                     │                      │
                     │                      └─ machine.SetEmotion(emotion)
                     ├─ TTS(Input: clean text)
                     └─ speak(): EventSpeak ──► render loop reads
                                               Snapshot().Emotion
                                               during StateSpeaking
                        speak() end: EventRest ──► emotion cleared
```

## Testing

- `emotion_test.go`: table-driven `ParseEmotion` cases — leading directive,
  embedded directive, no directive, unknown bracket left intact, multiple
  directives (first wins, all stripped), case-insensitivity, whitespace tidy.
- `state_test.go`: `SetEmotion`/`Snapshot().Emotion` round-trip; emotion cleared
  on `EventRest`.
- `voice_test.go`: `ProcessBatch`/`SpeakRemark` send the **cleaned** text to TTS
  and set the machine emotion from a directive; `currentSystemPrompt()` includes
  the protocol block appended to the persona.
- `EmotionVocabulary` membership: every entry resolves to a real face via
  `face.Canonical` (guards against advertising a nonexistent expression).

## Out of scope (future)

- Animation engine: mouth/face motion driven by emotion while speaking (would
  reconcile lip-sync with the held emotion face).
- Multiple emotions per utterance with positional timing.
- Idle-animation scheduler using the new faces (`internal/assistant/idle.go`).
- Settings UI for toggling LLM emotion directives.

## Verification

- `CGO_ENABLED=0 go test ./...` green (SDL packages excepted — they require
  CGO and are unrelated).
- `golangci-lint run ./...` adds no findings.
