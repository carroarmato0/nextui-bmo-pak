# Prompt Files Specification

**Date:** 2026-06-11
**Status:** Approved
**Scope:** Move the chat persona and TTS speaking-style prompts out of config.json into plain-text sidecar files

## 1. Purpose

Multiline structured prompts are painful to maintain inside JSON (single line,
`\n` escapes). The persona and voice prompts move to dedicated plain-text
files that can be copy-pasted verbatim, while config.json stays concise.

## 2. Files

Both live next to config.json in the pak HOME directory:

| File | Content | Built-in default |
|---|---|---|
| `persona.txt` | Chat system prompt (who BMO is) | `config.DefaultSystemPrompt` |
| `voice.txt` | TTS speaking-style instructions | `config.DefaultTTSInstructions` |

Path helpers: `config.PersonaPath(homeDir)`, `config.VoicePath(homeDir)`.

## 3. File lifecycle

`config.EnsurePromptFile(path, def string) (string, error)`:

- file missing → create it containing the default, return the default
- file exists but blank (whitespace-only) → write the default into it, return the default
- file has content → return the content unchanged; never overwrite

Both mains call this at startup for both files.

## 4. Config cleanup

- Remove `Config.SystemPrompt` (`system_prompt`) and `Provider.Instructions`
  (`instructions`) from the config schema; `Normalize()` no longer fills them.
- Old config.json files containing these keys keep loading fine — unknown
  JSON fields are ignored.
- `providers.SpeechRequest.Instructions` (the API field) is unchanged.

## 5. Runtime behavior

- The voice pipeline reads **both** prompts per utterance via source
  functions (`SetSystemPromptSource`, existing `SetTTSInstructionsSource`),
  so editing either file takes effect on the next spoken reply without a
  restart. Read errors fall back to the content loaded at startup.

## 6. Settings menu

New item **RESTORE DEFAULTS** (6th slot) in the settings overlay. Activating
it invokes a callback (wired in main) that unconditionally rewrites both
files with the built-in defaults. The label reflects prompt restoration; no
other config is touched.

## 7. Default persona update

`DefaultSystemPrompt` becomes the structured BMO persona (verbatim from the
user): identity (BMO, created by Moe, never an AI), personality ("grown man",
Football, skateboarding), environment (lives in a NextUI retro handheld),
language rules (1–3 spoken sentences, no markdown/emoji, occasional romanized
Korean, often offers to play a video game).

## 8. Test expectations

- `EnsurePromptFile`: missing → created with default; blank → default written;
  content → untouched and returned.
- Settings menu: RESTORE DEFAULTS item exists, activation fires the callback.
- Pipeline: system prompt source is consulted per utterance (mirror of the
  existing TTS instructions source test).
- Config: removed fields no longer round-trip; old JSON with the keys still loads.
