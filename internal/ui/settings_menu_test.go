package ui

import (
	"strings"
	"testing"

	"github.com/carroarmato0/nextui-bmo/internal/config"
)

// ── Structure ──────────────────────────────────────────────────────────────

func TestSettingsMenuHas17Items(t *testing.T) {
	m := NewSettingsMenu(config.Default())
	if got := len(m.Overlay().Items); got != 17 {
		t.Fatalf("expected 17 overlay items, got %d", got)
	}
}

func TestSettingsMenuItemCodes(t *testing.T) {
	m := NewSettingsMenu(config.Default())
	items := m.Overlay().Items
	want := []string{
		"log_level", "log_system_prompt", "mode",
		"stt_status", "chat_status", "tts_status", "voice_status",
		"aware_library", "aware_saves", "aware_playlog",
		"aware_system", "aware_achievements",
		"library_detail", "request_timeout", "proactive_talk", "mod", "restore_defaults",
	}
	for i, code := range want {
		if got := items[i].Code; got != code {
			t.Errorf("items[%d].Code = %q, want %q", i, got, code)
		}
	}
}

func TestSettingsMenuTimeoutCycles(t *testing.T) {
	m := NewSettingsMenu(config.Default())
	// Navigate to request_timeout at index 13.
	// From 0 (log_level): down → 2 (mode) → 7 (aware_library) → 8 → 9 → 10 → 11 → 12 → 13
	m.Move(1) // → 2
	m.Move(1) // → 7
	for i := 0; i < 6; i++ {
		m.Move(1)
	}
	if got := m.Overlay().Items[13].Code; got != "request_timeout" {
		t.Fatalf("expected request_timeout at focus after navigation, got %q", got)
	}
	initial := m.Config().RequestTimeout
	if err := m.ToggleFocused(); err != nil {
		t.Fatalf("ToggleFocused: %v", err)
	}
	if m.Config().RequestTimeout == initial {
		t.Fatal("timeout should have changed after toggle")
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
	// From MODE (2), down should jump to AWARE LIBRARY (7), skipping stt/chat/tts/voice (3-6).
	m.Move(1)
	if got := m.Overlay().Items[7].Focused; !got {
		t.Fatal("expected aware_library (idx 7) to be focused after Move(1) from mode")
	}
	// From AWARE LIBRARY (7), up should jump back to MODE (2).
	m.Move(-1)
	if got := m.Overlay().Items[2].Focused; !got {
		t.Fatal("expected mode (idx 2) to be focused after Move(-1) from aware_library")
	}
}

func TestSettingsMenuLogSystemPromptNotNavigableOutsideDebug(t *testing.T) {
	cfg := config.Default() // log_level = "info"
	m := NewSettingsMenu(cfg)
	// Cursor must never land on idx 1 (log_system_prompt) when not debug.
	for range 15 {
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
	// Move(5) from 0: (0+5)%15=5 (tts_status, skip)→6 (voice_status, skip)→7 (aware_library).
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
	// Move(4) from 7: (7+4)%15=11 (aware_achievements).
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
	m.Move(14) // proactive_talk is now at idx 14
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

	// restore_defaults is now at idx 16.
	menu.Move(16)
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
	m.Move(16) // restore_defaults is at idx 16
	if err := m.ToggleFocused(); err != nil {
		t.Fatalf("restore defaults: %v", err)
	}
	if !called {
		t.Fatal("restore defaults callback not invoked at focus 16")
	}
}

// ── AI Status Items ────────────────────────────────────────────────────────

func TestSettingsMenuAIStatusDisabledWhenIdle(t *testing.T) {
	cfg := config.Default() // Mode = "idle"
	m := NewSettingsMenu(cfg)
	items := m.Overlay().Items
	for _, idx := range []int{3, 4, 5, 6} {
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
	for _, idx := range []int{3, 4, 5, 6} {
		if items[idx].Disabled {
			t.Errorf("items[%d].Disabled = true, want false when mode is ai", idx)
		}
		if items[idx].Focused {
			t.Errorf("items[%d].Focused = true, want false (always non-navigable)", idx)
		}
	}
}

func TestSettingsMenuAIStatusShowsModelOnly(t *testing.T) {
	cfg := config.Default()
	cfg.STT = config.Provider{Name: "openai-compatible", Model: "whisper-1", APIKey: "sk-s"}
	cfg.Chat = config.Provider{Name: "openai-compatible", Model: "gpt-4o-mini"}
	cfg.TTS = config.Provider{Name: "openai-compatible", Model: "tts-1", Voice: "nova", APIKey: "sk-t"}
	m := NewSettingsMenu(cfg)
	items := m.Overlay().Items
	if got := items[3].Label; got != "STT: whisper-1" {
		t.Errorf("stt_status label = %q, want %q", got, "STT: whisper-1")
	}
	if got := items[4].Label; got != "CHAT: gpt-4o-mini" {
		t.Errorf("chat_status label = %q, want %q", got, "CHAT: gpt-4o-mini")
	}
	if got := items[5].Label; got != "TTS: tts-1" {
		t.Errorf("tts_status label = %q, want %q", got, "TTS: tts-1")
	}
	if got := items[6].Label; got != "VOICE: nova" {
		t.Errorf("voice_status label = %q, want %q", got, "VOICE: nova")
	}
}

func TestSettingsMenuVoiceStatusNotSetWhenNoVoice(t *testing.T) {
	cfg := config.Default()
	cfg.TTS = config.Provider{Name: "openai-compatible", Model: "tts-1"}
	m := NewSettingsMenu(cfg)
	if got := m.Overlay().Items[6].Label; got != "VOICE: NOT SET" {
		t.Errorf("voice_status label = %q, want %q", got, "VOICE: NOT SET")
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
	if len(overlay.Items) != 17 {
		t.Fatalf("expected 17 overlay items, got %d", len(overlay.Items))
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

func TestSettingsMenuOverlayHasSubtitleAndFooter(t *testing.T) {
	m := NewSettingsMenu(config.Default())
	overlay := m.Overlay()
	if len(overlay.Subtitle) == 0 {
		t.Fatal("Settings overlay should include navigation hints in Subtitle")
	}
	if overlay.Footer == "" {
		t.Fatal("Settings overlay should include close hint in Footer")
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

func TestSettingsMenuCyclesMod(t *testing.T) {
	m := NewSettingsMenu(config.Default())
	m.SetModChoices([]ModChoice{
		{ID: "default", Label: "BMO (DEFAULT)"},
		{ID: "evil", Label: "EVIL BMO"},
	})
	var changed string
	m.SetModChangeCallback(func(id string) { changed = id })

	m.Move(15) // mod selector
	if got := m.Overlay().Items[15].Code; got != "mod" {
		t.Fatalf("expected mod item at idx 15, got %q", got)
	}
	if err := m.ToggleFocused(); err != nil { // default -> evil
		t.Fatalf("ToggleFocused: %v", err)
	}
	if m.Config().ActiveMod != "evil" {
		t.Fatalf("ActiveMod = %q, want evil", m.Config().ActiveMod)
	}
	if changed != "evil" {
		t.Fatalf("callback got %q, want evil", changed)
	}
	if got := m.Overlay().Items[15].Label; got != "MOD: EVIL BMO" {
		t.Fatalf("mod item label = %q, want %q", got, "MOD: EVIL BMO")
	}
	if err := m.ToggleFocused(); err != nil { // evil -> default (wraps)
		t.Fatalf("ToggleFocused: %v", err)
	}
	if m.Config().ActiveMod != "default" {
		t.Fatalf("ActiveMod = %q, want default after wrap", m.Config().ActiveMod)
	}
}

// A factory-fresh config has ActiveMod == "" (no mod chosen yet). The MOD item
// must display the default entry's label, and the first cycle must advance to
// the next mod rather than appearing to do nothing — because the default mod is
// always the first choice, an unmatched "" resolves to that first entry.
func TestSettingsMenuModDefaultsToFirstChoiceWhenUnset(t *testing.T) {
	cfg := config.Default()
	if cfg.ActiveMod != "" {
		t.Fatalf("precondition: default ActiveMod = %q, want empty", cfg.ActiveMod)
	}
	m := NewSettingsMenu(cfg)
	m.SetModChoices([]ModChoice{
		{ID: "default", Label: "BMO (DEFAULT)"},
		{ID: "evil", Label: "EVIL BMO"},
	})

	if got := m.Overlay().Items[15].Label; got != "MOD: BMO (DEFAULT)" {
		t.Fatalf("unset ActiveMod label = %q, want %q", got, "MOD: BMO (DEFAULT)")
	}

	m.Move(15)
	if err := m.ToggleFocused(); err != nil {
		t.Fatalf("ToggleFocused: %v", err)
	}
	if m.Config().ActiveMod != "evil" {
		t.Fatalf("first cycle from unset = %q, want evil", m.Config().ActiveMod)
	}
}
