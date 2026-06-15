# BMO Mod Emotion Vocabulary — Design Spec

**Date:** 2026-06-15
**Status:** Approved (design)
**Builds on:** `2026-06-15-bmo-mods-foundation-design.md` (the `mods/` foundation
and the `SelfContained()` rule).

## Motivation

The mods foundation lets a named mod ship its own face set, but the LLM emotion
protocol still injects a hardcoded 28-word BMO vocabulary
(`internal/assistant/emotion.go`). A self-contained mod with a different face
set would have BMO emit `[emotion]` directives for faces it does not have — they
fold to the mod's neutral and the LLM is effectively told about emotions that do
not exist.

This spec makes the emotion vocabulary **derive from the active mod**: the LLM is
told exactly which emotions the mod supports, can be given a human description per
emotion to choose well, and — crucially — a mod can introduce **brand-new emotion
names** (e.g. `grumpy`, `ecstatic`) that render from its own SVGs.

## Scope

Two cleanly separable concerns, delivered together because the vocabulary must
only advertise renderable faces:

1. **Render pipeline** — let `face.Cache` draw an arbitrary custom-named SVG from
   a self-contained mod's `faces/` directory (today such names fold to `neutral`).
2. **Vocabulary + hinting** — derive the LLM emotion protocol from the active
   mod's faces, with optional per-emotion descriptions from `mod.json`, and parse
   the resulting directives (custom or known).

## Render Pipeline (`internal/face`)

**Problem:** `Cache.Frame` calls `Canonical(expr)` and keys frames by the
canonical name. `Canonical` only knows the fixed name set, so a custom name like
`grumpy` folds to `neutral` and the custom SVG never renders.

**Change — `Library.Resolve` (Approach A):**
- New method `Library.Resolve(expr string) string`. It lowercases/trims `expr`,
  and returns the **raw name** when (a) the raw name matches the existing
  safe-filename pattern (`fileNameRe`, `^[a-z0-9_-]+$`) and (b) a disk file exists
  for it under the library's `faces/` directory (so a self-contained mod's
  `grumpy.svg` resolves to `"grumpy"`); otherwise it returns `Canonical(raw)`.
  Name-resolution authority lives in the Library, which already tries the raw
  filename — behind the same `fileNameRe` guard — before the canonical one in
  `Bytes`.
- `Cache.Frame` keys by `lib.Resolve(expr)` instead of calling `Canonical`
  directly. The on-demand rasterization path is unchanged, so custom names render
  on first request.
- `Cache.Warm` additionally pre-rasterizes the active mod's disk emotion faces
  (via `EmotionFaceNamesInDir`, below) so a custom face does not stutter on first
  use on the device.

**Behavior preservation:** For the default mod (not self-contained), known names
and aliases still flow through `Canonical` exactly as today — `Resolve` only
returns a raw name when a matching disk file exists, and for the default mod the
cache key for a known name equals its canonical name regardless. The only changed
behavior is that previously-unrenderable custom names now render.

**Name classification (removes the hand-maintained 28-word list):**
- `face.FunctionalNames` = `{blink, listening, thinking, speaking, sleeping}` —
  state-driven faces that are never emotions. The LLM can never request them;
  they remain overridable as art via the normal override mechanism.
- `face.EmotionNames()` = `CanonicalNames` minus `FunctionalNames` → the built-in
  emotion set (the current 28), now *derived* rather than hand-listed.
- `face.EmotionFaceNamesInDir(dir string) []string` = the `*.svg` files in `dir`
  whose base name is not in `FunctionalNames`, returned sorted. One helper, reused
  by `Cache.Warm` and by vocabulary derivation.

## Vocabulary Model

One rule produces the vocabulary for both mod kinds:

```
base  = face.EmotionNames()                          // when NOT self-contained
base  = []                                           // when self-contained
disk  = face.EmotionFaceNamesInDir(mod.FacesDir())   // custom + known svgs the mod ships
vocab = dedupe(base ++ disk)                          // base in canonical order, then new disk names
```

- Self-contained `evil` shipping `happy.svg`, `grumpy.svg` → `{happy, grumpy}`.
- `default` with nothing extra → the built-in 28.
- `default` (or any embedded-inheriting mod) that adds `grumpy.svg` → `28 + grumpy`.
- A named mod with no faces (inherits embedded) → the built-in 28.

Dedupe is by name, preserving first occurrence, so a self-contained mod shipping
`angry.svg` lists `angry` once, and a default overlay overriding `angry.svg` does
not double-list it.

## Manifest Hints (`internal/mod`)

Add to `Manifest`:

```go
Emotions map[string]string `json:"emotions,omitempty"` // emotion name -> LLM description
```

```json
{
  "name": "Evil BMO",
  "emotions": {
    "grumpy": "sulky and irritable",
    "ecstatic": "overjoyed, can barely contain it"
  }
}
```

- Descriptions are optional and apply to any name, custom or known. A face with
  no entry is still advertised as a bare word.
- Parsing stays tolerant: a missing or malformed `mod.json` yields no descriptions
  and the mod still loads (existing `LoadManifest` behavior).

## Dynamic Protocol + Parser (`internal/assistant`)

- New `EmotionEntry{ Name string; Description string }`.
- New `BuildEmotionVocabulary(builtin, disk []string, descriptions map[string]string) []EmotionEntry`
  — applies the dedupe/ordering rule and attaches descriptions. `main.go` passes
  `builtin = nil` for a self-contained mod, else `face.EmotionNames()`.
- `emotionProtocolPrompt(entries []EmotionEntry) string` renders each entry as a
  bare word, or `word — description` when a description is present.
- The pipeline gains `SetEmotionVocabularySource(func() []EmotionEntry)`, mirroring
  `SetSystemPromptSource`. It is consulted per-utterance, so the advertised
  vocabulary always reflects the active mod and updates on a live mod switch.
- `ParseEmotion(reply string, valid map[string]Expression) (string, Expression)`
  takes the active name set instead of consulting a package global. The matched
  name (custom or known) becomes the `Expression` that flows through
  `machine.SetEmotion` → `FrameState.Expression` → `Cache.Frame`, where `Resolve`
  renders the mod's SVG. The directive regex widens from `[A-Za-z_]+` to
  `[A-Za-z0-9_-]+` to match the face-filename charset (`^[a-z0-9_-]+$`).
- The hardcoded 28-entry `EmotionVocabulary` global is removed.
  `TestEmotionVocabularyResolvesToItself` adapts to iterate `face.EmotionNames()`.

## Wiring (`cmd/bmo-pak`)

- Build the emotion-vocabulary source from `activeMod` and its self-contained
  flag, and install it on the pipeline via `SetEmotionVocabularySource`. The
  source computes `BuildEmotionVocabulary(builtin, disk, descriptions)` from the
  current `activeMod` on each call.
- `reloadMod` already re-points the per-utterance source closures on a mod switch;
  because the vocabulary source reads the current `activeMod`, switching mods
  updates what the LLM is told with no restart.

## Testing

- **`face`**: `Resolve` returns the raw name for a custom self-contained face and
  the canonical name otherwise; `Cache.Frame` renders a custom-named SVG;
  `EmotionFaceNamesInDir` excludes functional faces; `EmotionNames()` equals
  `CanonicalNames` minus `FunctionalNames`.
- **`mod`**: `Manifest.Emotions` parses; missing/malformed `mod.json` tolerated.
- **`assistant`**: `BuildEmotionVocabulary` for self-contained vs overlay, dedupe,
  ordering, and description attachment; `emotionProtocolPrompt` format with and
  without descriptions; `ParseEmotion` accepts custom names, rejects unknown
  bracketed words, and the widened regex matches `a-z0-9_-`.
- **`cmd/bmo-pak`**: no unit tests (by design); verified by `CGO_ENABLED=0 go build`,
  the full suite, `golangci-lint run ./...`, and a manual mod-switch check that the
  protocol reflects the new mod.

## Out of Scope / Deferred

- **Declarative animation engine + generalized lip-sync** — still deferred to its
  own spec.
- **Mod-controlled TTS voice/provider** — today a mod can shape *how* BMO sounds
  via the `voice.txt` style prompt, but not the actual synthesized voice. Letting
  a mod choose the TTS voice/model or a different provider (e.g. OpenAI `nova` vs.
  ElevenLabs) implies per-mod provider config, a provider abstraction, and
  credential handling. Explicitly deferred to a later spec.
- **Prompt-injection hardening of descriptions** — a mod's emotion descriptions
  enter the LLM system prompt. This is the same trust model as the existing
  `persona.txt`: user-installed mods are trusted. Descriptions are not sanitized;
  noted as an accepted decision, not a gap.
