package ui

import (
	"testing"

	"github.com/carroarmato0/nextui-bmo/internal/config"
)

func TestSettingsMenuEditsAPIKeys(t *testing.T) {
	menu := NewSettingsMenu(config.Default())
	if got := menu.Title(); got != "SETTINGS" {
		t.Fatalf("Title() = %q, want SETTINGS", got)
	}
	if got := menu.Overlay().Items[0].Code; got != "log_level" {
		t.Fatalf("first item code = %q, want log_level", got)
	}
	if got := menu.Overlay().Items[1].Code; got != "mode" {
		t.Fatalf("second item code = %q, want mode", got)
	}

	// Move to mode (position 1) and toggle it
	menu.Move(1)
	if err := menu.ToggleFocused(); err != nil {
		t.Fatalf("ToggleFocused() error = %v", err)
	}
	if got := menu.Config().Mode; got != config.ModeAI {
		t.Fatalf("Mode = %q, want ai", got)
	}

	menu.SetProvider("stt", config.Provider{Name: "openai-compatible", Model: "whisper-1"})
	menu.SetProvider("chat", config.Provider{Name: "openai-compatible", Model: "gpt-4o-mini"})
	menu.SetProvider("tts", config.Provider{Name: "openai-compatible", Model: "tts-1", Voice: "alloy"})

	// Move to STT (position 2)
	menu.Move(1)
	if err := menu.ToggleFocused(); err != nil {
		t.Fatalf("ToggleFocused() enter edit error = %v", err)
	}
	if !menu.IsEditing() || menu.EditingKind() != "stt" {
		t.Fatalf("expected stt edit mode, got editing=%v kind=%q", menu.IsEditing(), menu.EditingKind())
	}
	menu.InsertText("sk-stt")
	if err := menu.SubmitEdit(); err != nil {
		t.Fatalf("SubmitEdit() error = %v", err)
	}
	if got := menu.Config().STT.APIKey; got != "sk-stt" {
		t.Fatalf("STT.APIKey = %q, want sk-stt", got)
	}

	// Move to Chat (position 3)
	menu.Move(1)
	if err := menu.ToggleFocused(); err != nil {
		t.Fatalf("ToggleFocused() enter edit error = %v", err)
	}
	menu.InsertText("sk-chat")
	if err := menu.SubmitEdit(); err != nil {
		t.Fatalf("SubmitEdit() error = %v", err)
	}
	if got := menu.Config().Chat.APIKey; got != "sk-chat" {
		t.Fatalf("Chat.APIKey = %q, want sk-chat", got)
	}

	// Move to TTS (position 4)
	menu.Move(1)
	if err := menu.ToggleFocused(); err != nil {
		t.Fatalf("ToggleFocused() enter edit error = %v", err)
	}
	menu.InsertText("sk-tts")
	if err := menu.SubmitEdit(); err != nil {
		t.Fatalf("SubmitEdit() error = %v", err)
	}
	if got := menu.Config().TTS.APIKey; got != "sk-tts" {
		t.Fatalf("TTS.APIKey = %q, want sk-tts", got)
	}

	saved, err := menu.Save()
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if !saved.SetupComplete {
		t.Fatal("Save() should mark setup complete")
	}
}

func TestSettingsMenuLogLevelCycles(t *testing.T) {
	cfg := config.Default() // LogLevel = "info"
	m := NewSettingsMenu(cfg)

	var gotLevel string
	m.SetLogLevelCallback(func(l string) { gotLevel = l })

	// Focus 0 = log level; cycle info → warn
	m.ToggleFocused()
	if m.Config().LogLevel != "warn" {
		t.Fatalf("expected warn, got %s", m.Config().LogLevel)
	}
	if gotLevel != "warn" {
		t.Fatalf("callback not called: gotLevel=%s", gotLevel)
	}

	// Cycle warn → error
	m.ToggleFocused()
	if m.Config().LogLevel != "error" {
		t.Fatalf("expected error, got %s", m.Config().LogLevel)
	}
}

func TestSettingsMenuModeToggles(t *testing.T) {
	cfg := config.Default() // Mode = "idle"
	m := NewSettingsMenu(cfg)
	m.Move(1) // focus = 1 (mode item)

	m.ToggleFocused() // idle → ai
	if m.Config().Mode != config.ModeAI {
		t.Fatalf("expected ai, got %s", m.Config().Mode)
	}

	m.ToggleFocused() // ai → idle
	if m.Config().Mode != config.ModeIdle {
		t.Fatalf("expected idle, got %s", m.Config().Mode)
	}
}

func TestSettingsMenuRestoreDefaults(t *testing.T) {
	menu := NewSettingsMenu(config.Default())

	restored := 0
	menu.SetRestoreDefaultsCallback(func() error {
		restored++
		return nil
	})

	// The item must be present in the overlay.
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

	// Move to the restore item (position 11) and activate it.
	menu.Move(11)
	if err := menu.ToggleFocused(); err != nil {
		t.Fatalf("ToggleFocused() error = %v", err)
	}
	if restored != 1 {
		t.Fatalf("restore callback fired %d times, want 1", restored)
	}

	// Without a callback the item is inert but not an error.
	menu.SetRestoreDefaultsCallback(nil)
	if err := menu.ToggleFocused(); err != nil {
		t.Fatalf("ToggleFocused() without callback error = %v", err)
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

func TestSettingsMenuTogglesAwarenessCategories(t *testing.T) {
	cfg := config.Default()
	m := NewSettingsMenu(cfg)
	// Focus 5 = Awareness: Library (defaults on → toggles off).
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
	// Focus 9 = Awareness: Achievements.
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
	m.Move(10)
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

func TestSettingsMenuRestoreDefaultsMovedToLastSlot(t *testing.T) {
	m := NewSettingsMenu(config.Default())
	called := false
	m.SetRestoreDefaultsCallback(func() error { called = true; return nil })
	m.Move(11)
	if err := m.ToggleFocused(); err != nil {
		t.Fatalf("restore defaults: %v", err)
	}
	if !called {
		t.Fatal("restore defaults callback not invoked at focus 11")
	}
}

func TestSettingsMenuOverlayShowsAwarenessItems(t *testing.T) {
	m := NewSettingsMenu(config.Default())
	overlay := m.Overlay()
	if len(overlay.Items) != 12 {
		t.Fatalf("expected 12 overlay items, got %d", len(overlay.Items))
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
