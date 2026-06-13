# AI Settings Indicators & Debug Logging Design

Date: 2026-06-13

## Overview

Three related improvements:
1. Replace editable AI provider fields in the Settings overlay with read-only status indicators that grey out when mode is IDLE.
2. Log the full LLM input (system prompt + TTS instructions) at DEBUG level in `ProcessBatch`.
3. Add a "LOG SYSTEM PROMPT" toggle to the Settings menu that gates whether system-prompt lines appear in debug output. The toggle is hidden unless log level is set to DEBUG, and defaults to OFF.

---

## Part 1 — AI Status Indicators in Settings

### Problem

The Settings menu contains editable STT/CHAT/TTS API key fields (currently indices 2–4). These are redundant with the dedicated AI SETUP menu (Y button) and imply the user can configure them in-place. They also remain active-looking even when mode is IDLE.

### Goal

- Show STT, CHAT, and TTS provider status as **read-only labels** (model + key set/missing) using the existing `providerSummaryLabel`.
- When mode is **IDLE**: items are **greyed out** (dimmed colors, `Disabled: true`).
- When mode is **AI**: items display in normal colors.
- The cursor **always skips** these items — never focused, never interactive.

---

## Part 2 — Full LLM Input Debug Logging

### Problem

At DEBUG log level the user transcript is visible, but the system prompt and TTS instructions sent to the providers are not, making it hard to diagnose persona, device context, or voice-style issues.

### Goal

In `ProcessBatch`, log system prompt and TTS instructions at DEBUG level. Combined with the already-logged transcript, this gives the complete picture of what reaches the LLM on each interaction. Controlled by a new `LogSystemPrompt` config field so the user can omit sensitive prompt text from logs.

---

## Part 3 — "LOG SYSTEM PROMPT" Settings Toggle

### Goal

Add a boolean config field `LogSystemPrompt` (default `false`). A toggle item in the Settings menu controls it. The item is **hidden** (not rendered, not navigable) when log level is not DEBUG, and visible only when the user selects DEBUG log level. This prevents prompt leakage in normal-mode logs.

---

## Changes

### `internal/config/config.go`

Add `LogSystemPrompt bool` field (JSON `"log_system_prompt,omitempty"`). Zero value is `false`. No change to `Default()` needed.

### `internal/ui/menu.go`

Add two new fields to `OverlayItem`:
- `Disabled bool` — item is rendered in dimmed colors; cursor always skips it.
- `Hidden bool` — item is not rendered at all; cursor always skips it.

### `internal/renderer/bmo.go`

Add `Disabled bool` and `Hidden bool` to `OverlayItem`.

In `drawOverlay`:
- Skip hidden items entirely (do not render, do not advance `top`).
- For disabled items, use dimmed colors and skip selected/focused decoration:
  - Box: `rgba{40, 65, 70, 255}`
  - Label: `rgba{95, 115, 115, 255}`

### `cmd/bmo-pak/main.go`

In `convertOverlay`, pass `Disabled` and `Hidden` through from `ui.OverlayItem` to `renderer.OverlayItem`.

In `commitMenu`, after the existing updates, call `audioPipeline.SetLogSystemPrompt(cfg.LogSystemPrompt)` when the pipeline is non-nil.

### `internal/ui/settings_menu.go`

New item layout (count = 14):

| Index | Code | Behaviour |
|-------|------|-----------|
| 0 | `log_level` | cycle log level |
| 1 | `log_system_prompt` | toggle; **Hidden** when log level ≠ debug |
| 2 | `mode` | toggle IDLE/AI |
| 3 | `stt_status` | read-only indicator; **Disabled** when mode = IDLE; cursor always skips |
| 4 | `chat_status` | read-only indicator; same |
| 5 | `tts_status` | read-only indicator; same |
| 6 | `aware_library` | toggle |
| 7 | `aware_saves` | toggle |
| 8 | `aware_playlog` | toggle |
| 9 | `aware_system` | toggle |
| 10 | `aware_achievements` | toggle |
| 11 | `library_detail` | cycle |
| 12 | `proactive_talk` | cycle |
| 13 | `restore_defaults` | action |

**`Move`** — `count = 14`. After computing the candidate index, loop in the same direction while `shouldSkip()` returns true:
- Always skip indices 3, 4, 5 (AI status indicators).
- Skip index 1 when `m.cfg.LogLevel != "debug"`.

**`Overlay`** — build 14 items per the table above. Items 3–5 use `providerSummaryLabel`, always `Focused: false`, `Disabled: m.cfg.Mode != config.ModeAI`. Item 1 uses `"LOG SYSTEM PROMPT: " + onOff(m.cfg.LogSystemPrompt)`, `Hidden: strings.ToLower(m.cfg.LogLevel) != "debug"`.

**`ToggleFocused`** — switch on focus, cases 0–13 per table (cases 3, 4, 5 absent — unreachable):
- case 1: `m.cfg.LogSystemPrompt = !m.cfg.LogSystemPrompt`
- case 2: toggle mode
- cases 6–10: device context toggles
- case 11: cycle library detail
- case 12: cycle proactive talk
- case 13: restore defaults

**Remove dead editing code** — `editing`, `editingKind`, `draft` fields; `BeginAPIKeyEdit`, `SetAPIKey`, `currentAPIKey`, `InsertText`, `Backspace`, `SubmitEdit`, `CancelEdit`, `IsEditing`, `EditingKind`, `EditBuffer`. `SettingsMenu` will no longer satisfy the `editable` interface; the type-assertion in `main.go` returns `ok=false` cleanly.

### `internal/assistant/voice.go`

Add `logSystemPrompt bool` field to `VoicePipeline`.

Add `SetLogSystemPrompt(v bool)` method.

In `ProcessBatch`, before `chat.Reply`:
```go
if p.logger != nil && p.logSystemPrompt {
    p.logger.Debugf("pipeline system prompt: %q", p.currentSystemPrompt())
}
```

Before `tts.Speak`:
```go
if p.logger != nil && p.logSystemPrompt {
    p.logger.Debugf("pipeline TTS instructions: %q", p.currentTTSInstructions())
}
```

---

## Files Changed

| File | Change |
|------|--------|
| `internal/config/config.go` | Add `LogSystemPrompt bool` field |
| `internal/ui/menu.go` | Add `Disabled`, `Hidden` to `OverlayItem` |
| `internal/renderer/bmo.go` | Add `Disabled`, `Hidden`; dim/skip in `drawOverlay` |
| `cmd/bmo-pak/main.go` | Pass `Disabled`/`Hidden` in `convertOverlay`; update `commitMenu` |
| `internal/ui/settings_menu.go` | New layout (14 items); skip logic; remove editing code |
| `internal/assistant/voice.go` | `logSystemPrompt` flag; guarded debug logs in `ProcessBatch` |
