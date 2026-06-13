# AI Settings Indicators & Debug Logging Design

Date: 2026-06-13

## Overview

Two related improvements:
1. Replace editable AI provider fields in the Settings overlay with read-only status indicators that grey out when the mode is IDLE.
2. Log the full LLM input (system prompt + TTS instructions) at DEBUG level in `ProcessBatch`.

---

## Part 1 — AI Status Indicators in Settings

### Problem

The Settings menu currently contains editable STT/CHAT/TTS API key fields (indices 2–4). These are redundant with the dedicated AI SETUP menu (opened via Y) and create confusion by implying they can be configured in-place. They also remain active-looking even when mode is IDLE, where they have no effect.

### Goal

- Show STT, CHAT, and TTS provider status as **read-only labels** (model + key set/missing) in the Settings overlay.
- When mode is **IDLE**: these items are **greyed out** (dimmed colors) to signal they are inactive.
- When mode is **AI**: these items display in **normal colors** as informational status.
- The cursor **always skips** these items — they are never focused, never interactive.

### Changes

#### `internal/ui/menu.go`
Add `Disabled bool` to `OverlayItem`.

#### `internal/renderer/bmo.go`
Add `Disabled bool` to `OverlayItem`. In `drawOverlay`, when `item.Disabled`, render with dimmed colors:
- Box: `rgba{40, 65, 70, 255}` (vs normal `rgba{79, 139, 141, 255}`)
- Label: `rgba{95, 115, 115, 255}` (vs normal `rgba{214, 235, 227, 255}`)
No focus/selected state is ever rendered for a disabled item (those fields will always be false).

#### `cmd/bmo-pak/main.go` — `convertOverlay`
Pass `Disabled` through from `ui.OverlayItem` to `renderer.OverlayItem`.

#### `internal/ui/settings_menu.go`

**`Move`**: after computing the new focus index, skip over indices 2, 3, 4 by looping in the same direction until landing outside that range. `count` stays 13. This makes navigation jump directly between `MODE` (1) and `AWARE LIBRARY` (5).

**`Overlay`**: items 2–4 change from `providerKeyLabel(...)` to `providerSummaryLabel(...)`. They always have `Focused: false`. They get `Disabled: m.cfg.Mode != config.ModeAI`.

**`ToggleFocused`**: remove cases 2, 3, 4 (unreachable after `Move` change).

**Dead editing code removed**: fields `editing`, `editingKind`, `draft`; methods `BeginAPIKeyEdit`, `SetAPIKey`, `currentAPIKey`, `InsertText`, `Backspace`, `SubmitEdit`, `CancelEdit`, `IsEditing`, `EditingKind`, `EditBuffer`. `SettingsMenu` will no longer satisfy the `editable` interface used in `main.go`; the type assertion there cleanly returns `ok=false` and no code change is needed.

---

## Part 2 — Full LLM Input Debug Logging

### Problem

When log level is DEBUG, the logs show the user's transcript but not the system prompt or TTS instructions sent to the providers. This makes it hard to diagnose persona, device context, or voice style issues.

### Goal

In `ProcessBatch`, log at DEBUG level before each provider call:
- The resolved system prompt (before `chat.Reply`)
- The resolved TTS instructions (before `tts.Speak`)

Combined with the already-logged transcript, this gives the full picture of what is sent to the LLM on each interaction.

No changes to `SpeakRemark` or `SpeakVerbatim`.

### Changes

#### `internal/assistant/voice.go` — `ProcessBatch`

Before `p.chat.Reply(...)`:
```go
if p.logger != nil {
    p.logger.Debugf("pipeline system prompt: %q", p.currentSystemPrompt())
}
```

Before `p.tts.Speak(...)`:
```go
if p.logger != nil {
    p.logger.Debugf("pipeline TTS instructions: %q", p.currentTTSInstructions())
}
```

---

## Files Changed

| File | Change |
|------|--------|
| `internal/ui/menu.go` | Add `Disabled bool` to `OverlayItem` |
| `internal/renderer/bmo.go` | Add `Disabled bool` to `OverlayItem`; dim disabled items in `drawOverlay` |
| `cmd/bmo-pak/main.go` | Pass `Disabled` in `convertOverlay` |
| `internal/ui/settings_menu.go` | Skip AI slots in `Move`; read-only status labels; remove editing code |
| `internal/assistant/voice.go` | Debugf system prompt + TTS instructions in `ProcessBatch` |
