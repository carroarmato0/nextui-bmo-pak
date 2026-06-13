# AI Settings Indicators & Debug Logging Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace editable AI provider fields in the Settings overlay with read-only status indicators (greyed out when IDLE), add a guarded "LOG SYSTEM PROMPT" toggle that only appears in debug mode, and log the full LLM input (system prompt + TTS instructions) in `ProcessBatch` when the toggle is on.

**Architecture:** Three changes compose cleanly: (1) `OverlayItem` gains `Disabled`/`Hidden` fields used by the renderer; (2) `SettingsMenu` is restructured to 14 fixed slots where AI status items (3–5) are always cursor-skipped and the log-system-prompt item (1) is cursor-skipped unless log level is debug; (3) `VoicePipeline` gains a `logSystemPrompt` bool that gates two `Debugf` calls in `ProcessBatch`.

**Tech Stack:** Go, SDL2, existing `ui`/`renderer`/`assistant`/`config` packages; `go test ./...` for verification; `golangci-lint run ./...` after each commit.

---

## Task 1: Add `LogSystemPrompt` to Config

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write a failing test**

Add to `internal/config/config_test.go` (inside the existing `TestRoundTrip` function, after the existing `want` setup but before `Save`):

```go
want.LogSystemPrompt = true
```

And add a new assertion after the existing ones in `TestRoundTrip`:

```go
if got.LogSystemPrompt != want.LogSystemPrompt {
    t.Fatalf("LogSystemPrompt lost: got %v want %v", got.LogSystemPrompt, want.LogSystemPrompt)
}
```

- [ ] **Step 2: Run the test to confirm it fails**

```bash
go test ./internal/config/... -run TestRoundTrip -v
```

Expected: FAIL — `Config` has no `LogSystemPrompt` field.

- [ ] **Step 3: Add the field to Config**

In `internal/config/config.go`, add to the `Config` struct after `LibraryDetail`:

```go
LogSystemPrompt bool `json:"log_system_prompt,omitempty"`
```

No change to `Default()` needed — zero value (`false`) is the correct default.

- [ ] **Step 4: Run tests and lint**

```bash
go test ./internal/config/... -v
golangci-lint run ./internal/config/...
```

Expected: all PASS, no lint findings.

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): add LogSystemPrompt field"
```

---

## Task 2: Add `Disabled` and `Hidden` to `OverlayItem` + Wire Through Renderer

**Files:**
- Modify: `internal/ui/menu.go`
- Modify: `internal/renderer/bmo.go` (struct + `drawOverlay`)
- Modify: `cmd/bmo-pak/main.go` (`convertOverlay`)

- [ ] **Step 1: Add fields to `ui.OverlayItem`**

In `internal/ui/menu.go`, replace the `OverlayItem` struct:

```go
type OverlayItem struct {
	Code     string
	Label    string
	Selected bool
	Focused  bool
	Disabled bool
	Hidden   bool
}
```

- [ ] **Step 2: Add fields to `renderer.OverlayItem`**

In `internal/renderer/bmo.go`, replace the `OverlayItem` struct:

```go
type OverlayItem struct {
	Code     string
	Label    string
	Selected bool
	Focused  bool
	Disabled bool
	Hidden   bool
}
```

- [ ] **Step 3: Handle `Hidden` and `Disabled` in `drawOverlay`**

In `internal/renderer/bmo.go`, replace the item-drawing loop inside `drawOverlay` (the `for _, item := range overlay.Items` block):

```go
for _, item := range overlay.Items {
    if item.Hidden {
        continue
    }
    if item.Disabled {
        r.fillRectColor(left, top+3, 10, 10, rgba{40, 65, 70, 255})
        r.drawText(left+20, top, 2, rgba{95, 115, 115, 255}, item.Label)
        top += 22
        continue
    }
    boxColor := rgba{79, 139, 141, 255}
    if item.Selected {
        boxColor = rgba{170, 232, 183, 255}
    }
    if item.Focused {
        boxColor = rgba{255, 241, 145, 255}
    }
    r.fillRectColor(left, top+3, 10, 10, boxColor)
    if item.Selected {
        r.drawLine(left+2, top+8, left+4, top+11, rgba{16, 49, 56, 255})
        r.drawLine(left+4, top+11, left+8, top+3, rgba{16, 49, 56, 255})
    }
    labelColor := rgba{214, 235, 227, 255}
    if item.Focused {
        labelColor = rgba{255, 241, 145, 255}
    }
    r.drawText(left+20, top, 2, labelColor, item.Label)
    top += 22
}
```

- [ ] **Step 4: Pass `Disabled` and `Hidden` through `convertOverlay`**

In `cmd/bmo-pak/main.go`, update `convertOverlay`:

```go
func convertOverlay(src ui.OverlayState) *renderer.OverlayState {
	if !src.Visible {
		return nil
	}
	items := make([]renderer.OverlayItem, 0, len(src.Items))
	for _, item := range src.Items {
		items = append(items, renderer.OverlayItem{
			Code:     item.Code,
			Label:    item.Label,
			Selected: item.Selected,
			Focused:  item.Focused,
			Disabled: item.Disabled,
			Hidden:   item.Hidden,
		})
	}
	return &renderer.OverlayState{
		Visible:  true,
		Title:    src.Title,
		Subtitle: append([]string(nil), src.Subtitle...),
		Items:    items,
		Footer:   src.Footer,
	}
}
```

- [ ] **Step 5: Run tests and lint**

```bash
go test ./... -v
golangci-lint run ./...
```

Expected: all PASS, no new lint findings.

- [ ] **Step 6: Commit**

```bash
git add internal/ui/menu.go internal/renderer/bmo.go cmd/bmo-pak/main.go
git commit -m "feat(ui): add Disabled and Hidden fields to OverlayItem"
```

---

## Task 3: Refactor `SettingsMenu` to New 14-Item Layout

**Files:**
- Modify: `internal/ui/settings_menu.go`
- Modify: `internal/ui/settings_menu_test.go`

The new layout (14 items, fixed indices):

| idx | Code | Always navigable? |
|-----|------|-------------------|
| 0 | `log_level` | yes |
| 1 | `log_system_prompt` | only when `log_level == debug` |
| 2 | `mode` | yes |
| 3 | `stt_status` | never (read-only indicator) |
| 4 | `chat_status` | never (read-only indicator) |
| 5 | `tts_status` | never (read-only indicator) |
| 6 | `aware_library` | yes |
| 7 | `aware_saves` | yes |
| 8 | `aware_playlog` | yes |
| 9 | `aware_system` | yes |
| 10 | `aware_achievements` | yes |
| 11 | `library_detail` | yes |
| 12 | `proactive_talk` | yes |
| 13 | `restore_defaults` | yes |

- [ ] **Step 1: Write failing tests that capture the new behavior**

Replace the entire content of `internal/ui/settings_menu_test.go` with:

```go
package ui

import (
	"strings"
	"testing"

	"github.com/carroarmato0/nextui-bmo/internal/config"
)

// ── Structure ──────────────────────────────────────────────────────────────

func TestSettingsMenuHas14Items(t *testing.T) {
	m := NewSettingsMenu(config.Default())
	if got := len(m.Overlay().Items); got != 14 {
		t.Fatalf("expected 14 overlay items, got %d", got)
	}
}

func TestSettingsMenuItemCodes(t *testing.T) {
	m := NewSettingsMenu(config.Default())
	items := m.Overlay().Items
	want := []string{
		"log_level", "log_system_prompt", "mode",
		"stt_status", "chat_status", "tts_status",
		"aware_library", "aware_saves", "aware_playlog",
		"aware_system", "aware_achievements",
		"library_detail", "proactive_talk", "restore_defaults",
	}
	for i, code := range want {
		if got := items[i].Code; got != code {
			t.Errorf("items[%d].Code = %q, want %q", i, got, code)
		}
	}
}

// ── Navigation ─────────────────────────────────────────────────────────────

func TestSettingsMenuMoveSkipsAIStatusItems(t *testing.T) {
	m := NewSettingsMenu(config.Default())
	// From LOG LEVEL (0), down should jump straight to MODE (2), skipping idx 1 (not debug).
	m.Move(1)
	if got := m.Overlay().Items[2].Focused; !got {
		t.Fatal("expected mode item (idx 2) to be focused after Move(1) from log_level in non-debug mode")
	}
	// From MODE (2), down should jump to AWARE LIBRARY (6), skipping stt/chat/tts (3-5).
	m.Move(1)
	if got := m.Overlay().Items[6].Focused; !got {
		t.Fatal("expected aware_library (idx 6) to be focused after Move(1) from mode")
	}
	// From AWARE LIBRARY (6), up should jump back to MODE (2).
	m.Move(-1)
	if got := m.Overlay().Items[2].Focused; !got {
		t.Fatal("expected mode (idx 2) to be focused after Move(-1) from aware_library")
	}
}

func TestSettingsMenuLogSystemPromptNotNavigableOutsideDebug(t *testing.T) {
	cfg := config.Default() // log_level = "info"
	m := NewSettingsMenu(cfg)
	// Cursor must never land on idx 1 (log_system_prompt) when not debug.
	// Check by moving down from 0 repeatedly.
	for range 14 {
		m.Move(1)
		if m.Overlay().Items[1].Focused {
			t.Fatal("log_system_prompt item was focused while log level is not debug")
		}
	}
}

func TestSettingsMenuLogSystemPromptNavigableInDebug(t *testing.T) {
	cfg := config.Default()
	cfg.LogLevel = "debug"
	m := NewSettingsMenu(cfg)
	// From 0, Move(1) should land on idx 1 (log_system_prompt).
	m.Move(1)
	if !m.Overlay().Items[1].Focused {
		t.Fatal("log_system_prompt item should be focused after Move(1) when log level is debug")
	}
}

// ── Toggling ───────────────────────────────────────────────────────────────

func TestSettingsMenuLogLevelCycles(t *testing.T) {
	cfg := config.Default() // LogLevel = "info"
	m := NewSettingsMenu(cfg)

	var gotLevel string
	m.SetLogLevelCallback(func(l string) { gotLevel = l })

	m.ToggleFocused() // info → warn
	if m.Config().LogLevel != "warn" {
		t.Fatalf("expected warn, got %s", m.Config().LogLevel)
	}
	if gotLevel != "warn" {
		t.Fatalf("callback not called: gotLevel=%s", gotLevel)
	}
	m.ToggleFocused() // warn → error
	if m.Config().LogLevel != "error" {
		t.Fatalf("expected error, got %s", m.Config().LogLevel)
	}
}

func TestSettingsMenuLogSystemPromptToggles(t *testing.T) {
	cfg := config.Default()
	cfg.LogLevel = "debug"
	m := NewSettingsMenu(cfg)
	m.Move(1) // focus = 1 (log_system_prompt, accessible in debug)

	if m.Config().LogSystemPrompt {
		t.Fatal("LogSystemPrompt should default to false")
	}
	if err := m.ToggleFocused(); err != nil {
		t.Fatalf("ToggleFocused() error = %v", err)
	}
	if !m.Config().LogSystemPrompt {
		t.Fatal("LogSystemPrompt should be true after toggle")
	}
	if err := m.ToggleFocused(); err != nil {
		t.Fatalf("ToggleFocused() error = %v", err)
	}
	if m.Config().LogSystemPrompt {
		t.Fatal("LogSystemPrompt should be false after second toggle")
	}
}

func TestSettingsMenuModeToggles(t *testing.T) {
	cfg := config.Default() // Mode = "idle"
	m := NewSettingsMenu(cfg)
	m.Move(1) // skips idx 1 (not debug), lands on idx 2 (mode)

	m.ToggleFocused() // idle → ai
	if m.Config().Mode != config.ModeAI {
		t.Fatalf("expected ai, got %s", m.Config().Mode)
	}
	m.ToggleFocused() // ai → idle
	if m.Config().Mode != config.ModeIdle {
		t.Fatalf("expected idle, got %s", m.Config().Mode)
	}
}

func TestSettingsMenuTogglesAwarenessCategories(t *testing.T) {
	cfg := config.Default()
	m := NewSettingsMenu(cfg)
	// Move(5) from 0: (0+5)%14=5 (tts_status, skip+1)→6 (aware_library).
	m.Move(5)
	if err := m.ToggleFocused(); err != nil {
		t.Fatalf("toggle library: %v", err)
	}
	if m.Config().DeviceContext.Library {
		t.Fatal("library toggle did not flip off")
	}
	if err := m.ToggleFocused(); err != nil {
		t.Fatalf("toggle library back: %v", err)
	}
	if !m.Config().DeviceContext.Library {
		t.Fatal("library toggle did not flip back on")
	}
	// Move(4) from 6: (6+4)%14=10 (aware_achievements).
	m.Move(4)
	if err := m.ToggleFocused(); err != nil {
		t.Fatalf("toggle achievements: %v", err)
	}
	if m.Config().DeviceContext.Achievements {
		t.Fatal("achievements toggle did not flip off")
	}
}

func TestSettingsMenuCyclesProactiveTalk(t *testing.T) {
	m := NewSettingsMenu(config.Default())
	m.Move(12) // proactive_talk is now at idx 12
	want := []string{
		config.ProactiveChatty, config.ProactiveRegular,
		config.ProactiveOccasional, config.ProactiveRare, config.ProactiveOff,
	}
	for _, level := range want {
		if err := m.ToggleFocused(); err != nil {
			t.Fatalf("cycle proactive: %v", err)
		}
		if got := m.Config().ProactiveTalk; got != level {
			t.Fatalf("proactive talk = %q, want %q", got, level)
		}
	}
}

func TestSettingsMenuRestoreDefaults(t *testing.T) {
	menu := NewSettingsMenu(config.Default())

	restored := 0
	menu.SetRestoreDefaultsCallback(func() error {
		restored++
		return nil
	})

	overlay := menu.Overlay()
	found := false
	for _, item := range overlay.Items {
		if item.Code == "restore_defaults" {
			found = true
		}
	}
	if !found {
		t.Fatal("restore_defaults item missing from overlay")
	}

	// restore_defaults is now at idx 13.
	menu.Move(13)
	if err := menu.ToggleFocused(); err != nil {
		t.Fatalf("ToggleFocused() error = %v", err)
	}
	if restored != 1 {
		t.Fatalf("restore callback fired %d times, want 1", restored)
	}

	menu.SetRestoreDefaultsCallback(nil)
	if err := menu.ToggleFocused(); err != nil {
		t.Fatalf("ToggleFocused() without callback error = %v", err)
	}
}

func TestSettingsMenuRestoreDefaultsIsLastSlot(t *testing.T) {
	m := NewSettingsMenu(config.Default())
	called := false
	m.SetRestoreDefaultsCallback(func() error { called = true; return nil })
	m.Move(13) // restore_defaults is at idx 13
	if err := m.ToggleFocused(); err != nil {
		t.Fatalf("restore defaults: %v", err)
	}
	if !called {
		t.Fatal("restore defaults callback not invoked at focus 13")
	}
}

// ── AI Status Items ────────────────────────────────────────────────────────

func TestSettingsMenuAIStatusDisabledWhenIdle(t *testing.T) {
	cfg := config.Default() // Mode = "idle"
	m := NewSettingsMenu(cfg)
	items := m.Overlay().Items
	for _, idx := range []int{3, 4, 5} {
		if !items[idx].Disabled {
			t.Errorf("items[%d].Disabled = false, want true when mode is idle", idx)
		}
		if items[idx].Focused {
			t.Errorf("items[%d].Focused = true, want false (always non-navigable)", idx)
		}
	}
}

func TestSettingsMenuAIStatusEnabledWhenAI(t *testing.T) {
	cfg := config.Default()
	cfg.Mode = config.ModeAI
	m := NewSettingsMenu(cfg)
	items := m.Overlay().Items
	for _, idx := range []int{3, 4, 5} {
		if items[idx].Disabled {
			t.Errorf("items[%d].Disabled = true, want false when mode is ai", idx)
		}
		if items[idx].Focused {
			t.Errorf("items[%d].Focused = true, want false (always non-navigable)", idx)
		}
	}
}

func TestSettingsMenuAIStatusShowsProviderSummary(t *testing.T) {
	cfg := config.Default()
	cfg.STT = config.Provider{Name: "openai-compatible", Model: "whisper-1", APIKey: "sk-s"}
	cfg.Chat = config.Provider{Name: "openai-compatible", Model: "gpt-4o-mini"}
	m := NewSettingsMenu(cfg)
	items := m.Overlay().Items
	if !strings.HasPrefix(items[3].Label, "STT:") {
		t.Errorf("stt_status label = %q, want STT: prefix", items[3].Label)
	}
	if !strings.Contains(items[3].Label, "whisper-1") {
		t.Errorf("stt_status label = %q, want model name", items[3].Label)
	}
	if !strings.Contains(items[3].Label, "KEY SET") {
		t.Errorf("stt_status label = %q, want KEY SET", items[3].Label)
	}
	if !strings.Contains(items[4].Label, "KEY MISSING") {
		t.Errorf("chat_status label = %q, want KEY MISSING", items[4].Label)
	}
}

// ── Log System Prompt Overlay ──────────────────────────────────────────────

func TestSettingsMenuLogSystemPromptHiddenWhenNotDebug(t *testing.T) {
	m := NewSettingsMenu(config.Default()) // LogLevel = "info"
	if !m.Overlay().Items[1].Hidden {
		t.Fatal("log_system_prompt item should be Hidden when log level is not debug")
	}
}

func TestSettingsMenuLogSystemPromptVisibleWhenDebug(t *testing.T) {
	cfg := config.Default()
	cfg.LogLevel = "debug"
	m := NewSettingsMenu(cfg)
	if m.Overlay().Items[1].Hidden {
		t.Fatal("log_system_prompt item should not be Hidden when log level is debug")
	}
}

func TestSettingsMenuLogSystemPromptLabelReflectsValue(t *testing.T) {
	cfg := config.Default()
	cfg.LogLevel = "debug"
	m := NewSettingsMenu(cfg)
	if label := m.Overlay().Items[1].Label; !strings.Contains(label, "OFF") {
		t.Fatalf("expected OFF in label, got %q", label)
	}
	cfg.LogSystemPrompt = true
	m2 := NewSettingsMenu(cfg)
	if label := m2.Overlay().Items[1].Label; !strings.Contains(label, "ON") {
		t.Fatalf("expected ON in label, got %q", label)
	}
}

// ── Overlay items ──────────────────────────────────────────────────────────

func TestSettingsMenuOverlayShowsAwarenessItems(t *testing.T) {
	m := NewSettingsMenu(config.Default())
	overlay := m.Overlay()
	if len(overlay.Items) != 14 {
		t.Fatalf("expected 14 overlay items, got %d", len(overlay.Items))
	}
	labels := map[string]string{}
	for _, item := range overlay.Items {
		labels[item.Code] = item.Label
	}
	for code, want := range map[string]string{
		"aware_library":      "AWARE LIBRARY: ON",
		"aware_saves":        "AWARE SAVES: ON",
		"aware_playlog":      "AWARE PLAY LOG: ON",
		"aware_system":       "AWARE SYSTEM: ON",
		"aware_achievements": "AWARE ACHIEVEMENTS: ON",
		"proactive_talk":     "PROACTIVE TALK: OFF",
	} {
		if labels[code] != want {
			t.Errorf("item %s label = %q, want %q", code, labels[code], want)
		}
	}
}

func TestSettingsMenuSave(t *testing.T) {
	menu := NewSettingsMenu(config.Default())
	saved, err := menu.Save()
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if !saved.SetupComplete {
		t.Fatal("Save() should mark setup complete")
	}
}

func TestSettingsScreenProviderSummaries(t *testing.T) {
	screen := NewSettingsScreen(config.Default())
	if got := screen.ProviderSummary("stt"); got != "STT: NOT SET" {
		t.Fatalf("ProviderSummary() = %q, want missing", got)
	}
	if err := screen.SetAPIKey("stt", "sk-1"); err != nil {
		t.Fatalf("SetAPIKey() error = %v", err)
	}
	if got := screen.ProviderSummary("stt"); got != "STT: NOT SET" {
		t.Fatalf("ProviderSummary() = %q, want still not set until model/provider exists", got)
	}
	if err := screen.SetPTTButtons([]string{"BTN_TL", "BTN_TR"}); err != nil {
		t.Fatalf("SetPTTButtons() error = %v", err)
	}
	if got := len(screen.PTTButtons()); got != 2 {
		t.Fatalf("PTTButtons() len = %d, want 2", got)
	}
}
```

- [ ] **Step 2: Run to confirm the tests fail**

```bash
go test ./internal/ui/... -run TestSettings -v 2>&1 | head -40
```

Expected: several FAIL — new tests reference fields/behaviour that don't exist yet.

- [ ] **Step 3: Rewrite `SettingsMenu` struct — remove editing fields**

In `internal/ui/settings_menu.go`, replace the struct definition:

```go
type SettingsMenu struct {
	title            string
	cfg              config.Config
	focus            int
	onLogLevelChange func(string)
	onRestore        func() error
}
```

- [ ] **Step 4: Replace `Move` with skip-aware implementation**

In `internal/ui/settings_menu.go`, replace `Move` and add `shouldSkip`:

```go
func (m *SettingsMenu) Move(delta int) {
	if m == nil {
		return
	}
	const count = 14
	step := 1
	if delta < 0 {
		step = -1
	}
	m.focus = ((m.focus + delta) % count + count) % count
	for m.shouldSkip(m.focus) {
		m.focus = (m.focus + step + count) % count
	}
}

func (m *SettingsMenu) shouldSkip(idx int) bool {
	if idx >= 3 && idx <= 5 {
		return true
	}
	if idx == 1 && strings.ToLower(strings.TrimSpace(m.cfg.LogLevel)) != "debug" {
		return true
	}
	return false
}
```

- [ ] **Step 5: Replace `ToggleFocused` with new switch**

In `internal/ui/settings_menu.go`, replace `ToggleFocused`:

```go
func (m *SettingsMenu) ToggleFocused() error {
	if m == nil {
		return fmt.Errorf("nil settings menu")
	}
	switch m.focus {
	case 0:
		curr := strings.ToLower(strings.TrimSpace(m.cfg.LogLevel))
		next := logLevelOrder[0]
		for i, l := range logLevelOrder {
			if l == curr {
				next = logLevelOrder[(i+1)%len(logLevelOrder)]
				break
			}
		}
		m.cfg.LogLevel = next
		if m.onLogLevelChange != nil {
			m.onLogLevelChange(next)
		}
	case 1:
		m.cfg.LogSystemPrompt = !m.cfg.LogSystemPrompt
	case 2:
		if m.cfg.Mode == config.ModeIdle {
			m.cfg.Mode = config.ModeAI
		} else {
			m.cfg.Mode = config.ModeIdle
		}
	case 6:
		m.cfg.DeviceContext.Library = !m.cfg.DeviceContext.Library
	case 7:
		m.cfg.DeviceContext.Saves = !m.cfg.DeviceContext.Saves
	case 8:
		m.cfg.DeviceContext.PlayLog = !m.cfg.DeviceContext.PlayLog
	case 9:
		m.cfg.DeviceContext.System = !m.cfg.DeviceContext.System
	case 10:
		m.cfg.DeviceContext.Achievements = !m.cfg.DeviceContext.Achievements
	case 11:
		if m.cfg.LibraryDetail == config.LibraryDetailRandom {
			m.cfg.LibraryDetail = config.LibraryDetailFull
		} else {
			m.cfg.LibraryDetail = config.LibraryDetailRandom
		}
	case 12:
		levels := config.SupportedProactiveTalkLevels()
		curr := strings.ToLower(strings.TrimSpace(m.cfg.ProactiveTalk))
		next := levels[0]
		for i, l := range levels {
			if l == curr {
				next = levels[(i+1)%len(levels)]
				break
			}
		}
		m.cfg.ProactiveTalk = next
	case 13:
		if m.onRestore != nil {
			return m.onRestore()
		}
	default:
		return fmt.Errorf("unsupported focus %d", m.focus)
	}
	return nil
}
```

- [ ] **Step 6: Replace `Overlay` with new 14-item implementation**

In `internal/ui/settings_menu.go`, replace `Overlay`:

```go
func (m *SettingsMenu) Overlay() OverlayState {
	isDebug := strings.ToLower(strings.TrimSpace(m.cfg.LogLevel)) == "debug"
	isAI := m.cfg.Mode == config.ModeAI
	items := []OverlayItem{
		{Code: "log_level", Label: "LOG: " + strings.ToUpper(m.cfg.LogLevel),
			Selected: true, Focused: m.focus == 0},
		{Code: "log_system_prompt", Label: "LOG SYSTEM PROMPT: " + onOff(m.cfg.LogSystemPrompt),
			Selected: m.cfg.LogSystemPrompt, Focused: m.focus == 1, Hidden: !isDebug},
		{Code: "mode", Label: "MODE: " + strings.ToUpper(m.cfg.Mode),
			Selected: true, Focused: m.focus == 2},
		{Code: "stt_status", Label: providerSummaryLabel("STT", m.cfg.STT), Disabled: !isAI},
		{Code: "chat_status", Label: providerSummaryLabel("CHAT", m.cfg.Chat), Disabled: !isAI},
		{Code: "tts_status", Label: providerSummaryLabel("TTS", m.cfg.TTS), Disabled: !isAI},
		{Code: "aware_library", Label: "AWARE LIBRARY: " + onOff(m.cfg.DeviceContext.Library),
			Selected: m.cfg.DeviceContext.Library, Focused: m.focus == 6},
		{Code: "aware_saves", Label: "AWARE SAVES: " + onOff(m.cfg.DeviceContext.Saves),
			Selected: m.cfg.DeviceContext.Saves, Focused: m.focus == 7},
		{Code: "aware_playlog", Label: "AWARE PLAY LOG: " + onOff(m.cfg.DeviceContext.PlayLog),
			Selected: m.cfg.DeviceContext.PlayLog, Focused: m.focus == 8},
		{Code: "aware_system", Label: "AWARE SYSTEM: " + onOff(m.cfg.DeviceContext.System),
			Selected: m.cfg.DeviceContext.System, Focused: m.focus == 9},
		{Code: "aware_achievements", Label: "AWARE ACHIEVEMENTS: " + onOff(m.cfg.DeviceContext.Achievements),
			Selected: m.cfg.DeviceContext.Achievements, Focused: m.focus == 10},
		{Code: "library_detail", Label: "LIBRARY DETAIL: " + strings.ToUpper(m.cfg.LibraryDetail),
			Selected: true, Focused: m.focus == 11},
		{Code: "proactive_talk", Label: "PROACTIVE TALK: " + strings.ToUpper(m.cfg.ProactiveTalk),
			Selected: true, Focused: m.focus == 12},
		{Code: "restore_defaults", Label: "RESTORE DEFAULTS", Focused: m.focus == 13},
	}
	return OverlayState{Visible: true, Title: m.title, Items: items}
}
```

- [ ] **Step 7: Replace `Save` — remove editing flush**

In `internal/ui/settings_menu.go`, replace `Save`:

```go
func (m *SettingsMenu) Save() (config.Config, error) {
	if m == nil {
		return config.Config{}, fmt.Errorf("nil settings menu")
	}
	cfg := m.cfg
	cfg.Normalize()
	cfg.SetupComplete = true
	if err := cfg.Validate(); err != nil {
		return config.Config{}, err
	}
	m.cfg = cfg
	return cfg, nil
}
```

- [ ] **Step 8: Delete all dead editing code from `SettingsMenu`**

Remove these methods entirely from `internal/ui/settings_menu.go`:
- `BeginAPIKeyEdit(kind string) error`
- `SetAPIKey(kind, key string) error`
- `currentAPIKey(kind string) string`
- `InsertText(text string)`
- `Backspace()`
- `SubmitEdit() error`
- `CancelEdit()`
- `IsEditing() bool`
- `EditingKind() string`
- `EditBuffer() string`

Also remove any `fmt` import that is no longer needed (keep it if `fmt.Errorf` is still used in `ToggleFocused` and `Save`).

- [ ] **Step 9: Run tests**

```bash
go test ./internal/ui/... -v
```

Expected: all PASS.

- [ ] **Step 10: Run full suite and lint**

```bash
go test ./... && golangci-lint run ./...
```

Expected: all PASS, no new findings.

- [ ] **Step 11: Commit**

```bash
git add internal/ui/settings_menu.go internal/ui/settings_menu_test.go
git commit -m "feat(ui): refactor SettingsMenu to 14-item layout with read-only AI indicators"
```

---

## Task 4: `VoicePipeline` — Guarded Debug Logging

**Files:**
- Modify: `internal/assistant/voice.go`
- Modify: `internal/assistant/voice_test.go`

- [ ] **Step 1: Write failing tests**

Add to `internal/assistant/voice_test.go`:

```go
func TestProcessBatchDoesNotLogSystemPromptByDefault(t *testing.T) {
	m := NewMachine()
	m.SetMode("ai")
	stt := &fakeProvider{transcript: "hello"}
	chat := &fakeProvider{reply: "hi"}
	tts := &fakeProvider{speech: make([]byte, 2400)}
	pipe := NewVoicePipeline(m, &fakeWriter{}, stt, chat, tts, "whisper-1", "gpt-4o-mini", "tts-1", "nova", "secret persona", 16000, 1)
	logger := &captureLogger{}
	pipe.SetLogger(logger)
	pipe.SetTTSInstructions("secret voice style")

	if err := pipe.ProcessBatch(context.Background(), []byte{0x00, 0x40, 0x00, 0x40}); err != nil {
		t.Fatalf("ProcessBatch() error = %v", err)
	}
	logs := logger.joined()
	if strings.Contains(logs, "secret persona") {
		t.Errorf("system prompt leaked into logs with logSystemPrompt=false: %q", logs)
	}
	if strings.Contains(logs, "secret voice style") {
		t.Errorf("TTS instructions leaked into logs with logSystemPrompt=false: %q", logs)
	}
}

func TestProcessBatchLogsSystemPromptWhenEnabled(t *testing.T) {
	m := NewMachine()
	m.SetMode("ai")
	stt := &fakeProvider{transcript: "hello"}
	chat := &fakeProvider{reply: "hi"}
	tts := &fakeProvider{speech: make([]byte, 2400)}
	pipe := NewVoicePipeline(m, &fakeWriter{}, stt, chat, tts, "whisper-1", "gpt-4o-mini", "tts-1", "nova", "", 16000, 1)
	pipe.SetSystemPromptSource(func() string { return "be bmo, the computer" })
	pipe.SetTTSInstructions("speak like bmo")
	logger := &captureLogger{}
	pipe.SetLogger(logger)
	pipe.SetLogSystemPrompt(true)

	if err := pipe.ProcessBatch(context.Background(), []byte{0x00, 0x40, 0x00, 0x40}); err != nil {
		t.Fatalf("ProcessBatch() error = %v", err)
	}
	logs := logger.joined()
	if !strings.Contains(logs, "pipeline system prompt:") || !strings.Contains(logs, "be bmo, the computer") {
		t.Errorf("system prompt not in logs: %q", logs)
	}
	if !strings.Contains(logs, "pipeline TTS instructions:") || !strings.Contains(logs, "speak like bmo") {
		t.Errorf("TTS instructions not in logs: %q", logs)
	}
}
```

- [ ] **Step 2: Run to confirm they fail**

```bash
go test ./internal/assistant/... -run "TestProcessBatchDoes\|TestProcessBatchLogs" -v
```

Expected: FAIL — `SetLogSystemPrompt` undefined.

- [ ] **Step 3: Add `logSystemPrompt` field and `SetLogSystemPrompt` method**

In `internal/assistant/voice.go`, add `logSystemPrompt bool` to the `VoicePipeline` struct after `ttsInstructions`:

```go
logSystemPrompt bool
```

Add method after `SetTTSInstructionsSource`:

```go
// SetLogSystemPrompt controls whether system prompt and TTS instructions are
// written to the debug log on each ProcessBatch call.
func (p *VoicePipeline) SetLogSystemPrompt(v bool) {
	if p != nil {
		p.logSystemPrompt = v
	}
}
```

- [ ] **Step 4: Add guarded Debugf calls in `ProcessBatch`**

In `internal/assistant/voice.go`, in `ProcessBatch`, immediately before the `p.chat.Reply(...)` call, add:

```go
if p.logger != nil && p.logSystemPrompt {
    p.logger.Debugf("pipeline system prompt: %q", p.currentSystemPrompt())
}
```

And immediately before the `p.tts.Speak(...)` call in `ProcessBatch`, add:

```go
if p.logger != nil && p.logSystemPrompt {
    p.logger.Debugf("pipeline TTS instructions: %q", p.currentTTSInstructions())
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/assistant/... -v
```

Expected: all PASS.

- [ ] **Step 6: Run full suite and lint**

```bash
go test ./... && golangci-lint run ./...
```

Expected: all PASS, no new findings.

- [ ] **Step 7: Commit**

```bash
git add internal/assistant/voice.go internal/assistant/voice_test.go
git commit -m "feat(assistant): add guarded debug logging for system prompt and TTS instructions"
```

---

## Task 5: Wire `LogSystemPrompt` Through `commitMenu`

**Files:**
- Modify: `cmd/bmo-pak/main.go`

- [ ] **Step 1: Update `commitMenu` to propagate the flag**

In `cmd/bmo-pak/main.go`, inside `commitMenu` (the `func(menu ui.Menu) error` closure), after the existing `proactive.SetInterval(...)` call, add:

```go
if audioPipeline != nil {
    audioPipeline.SetLogSystemPrompt(cfg.LogSystemPrompt)
}
```

- [ ] **Step 2: Run full suite and lint**

```bash
go test ./... && golangci-lint run ./...
```

Expected: all PASS, no new findings.

- [ ] **Step 3: Commit**

```bash
git add cmd/bmo-pak/main.go
git commit -m "feat: wire LogSystemPrompt through commitMenu to voice pipeline"
```

---

## Self-Review

### Spec coverage

| Spec requirement | Task |
|---|---|
| Read-only AI provider indicators in Settings | Task 3 (`Overlay`, AI status items 3-5) |
| AI indicators greyed when mode = IDLE | Task 2 (`Disabled` field) + Task 3 |
| Cursor always skips AI indicator items | Task 3 (`shouldSkip`, `Move`) |
| LOG SYSTEM PROMPT toggle hidden unless debug | Task 3 (item 1, `Hidden: !isDebug`) |
| LOG SYSTEM PROMPT defaults to OFF | Task 1 (zero value of bool) |
| Remove editable API key fields from Settings | Task 3 (editing code deleted) |
| Debug log system prompt in ProcessBatch | Task 4 |
| Debug log TTS instructions in ProcessBatch | Task 4 |
| Logging gated by `logSystemPrompt` flag | Task 4 (`SetLogSystemPrompt`) |
| Flag updated when settings saved | Task 5 (`commitMenu`) |

All spec requirements covered. No placeholders. Type names and method signatures are consistent across tasks.
