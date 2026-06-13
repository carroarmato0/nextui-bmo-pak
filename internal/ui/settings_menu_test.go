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
