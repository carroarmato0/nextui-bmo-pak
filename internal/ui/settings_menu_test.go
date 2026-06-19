package ui

import (
	"strings"
	"testing"

	"github.com/carroarmato0/nextui-bmo/internal/config"
)

// ── Structure ──────────────────────────────────────────────────────────────

func TestSettingsMenuHas21Items(t *testing.T) {
	m := NewSettingsMenu(config.Default())
	if got := len(m.Overlay().Items); got != 21 {
		t.Fatalf("expected 21 overlay items, got %d", got)
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
		"library_detail", "request_timeout", "proactive_talk",
		"wake_word", "continued_convo", "mod",
		"spacer", "restore_defaults", "about",
	}
	for i, code := range want {
		if got := items[i].Code; got != code {
			t.Errorf("items[%d].Code = %q, want %q", i, got, code)
		}
	}
}

func TestSettingsMenuTimeoutCycles(t *testing.T) {
	cfg := config.Default()
	cfg.Mode = config.ModeAI // request_timeout is an AI-only row
	m := NewSettingsMenu(cfg)
	m.focusForTest(13)
	if got := m.Overlay().Items[13].Code; got != "request_timeout" {
		t.Fatalf("expected request_timeout at idx 13, got %q", got)
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

func TestSettingsMenuMoveSkipsHiddenAIRowsInIdle(t *testing.T) {
	m := NewSettingsMenu(config.Default()) // idle, non-debug
	// From LOG LEVEL (0), down jumps to MODE (2), skipping the hidden idx 1.
	m.Move(1)
	if got := m.Overlay().Items[2].Focused; !got {
		t.Fatal("expected mode item (idx 2) to be focused after Move(1) from log_level in non-debug mode")
	}
	// From MODE (2), down skips every hidden AI row (3-14) and lands on MOD (15).
	m.Move(1)
	if got := m.Overlay().Items[17].Focused; !got {
		t.Fatal("expected mod (idx 17) to be focused after Move(1) from mode in idle mode")
	}
	// From MOD (15), up returns to MODE (2).
	m.Move(-1)
	if got := m.Overlay().Items[2].Focused; !got {
		t.Fatal("expected mode (idx 2) to be focused after Move(-1) from mod")
	}
}

func TestSettingsMenuMoveEntersAIRowsWhenAI(t *testing.T) {
	cfg := config.Default()
	cfg.Mode = config.ModeAI
	m := NewSettingsMenu(cfg)
	// From MODE (2), down enters the now-visible provider rows (3 = stt_status).
	m.focusForTest(2)
	m.Move(1)
	if got := m.Overlay().Items[3].Focused; !got {
		t.Fatal("expected stt_status (idx 3) focused after Move(1) from mode in AI mode")
	}
	// The voice row (6) is visible but read-only: stepping down from tts (5)
	// must skip it and reach aware_library (7).
	m.focusForTest(5)
	m.Move(1)
	if got := m.Overlay().Items[7].Focused; !got {
		t.Fatal("expected aware_library (idx 7) focused after Move(1) from tts_status (voice skipped)")
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
	cfg.Mode = config.ModeAI // awareness rows are AI-only
	m := NewSettingsMenu(cfg)
	m.focusForTest(7) // aware_library
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
	m.focusForTest(11) // aware_achievements
	if err := m.ToggleFocused(); err != nil {
		t.Fatalf("toggle achievements: %v", err)
	}
	if m.Config().DeviceContext.Achievements {
		t.Fatal("achievements toggle did not flip off")
	}
}

func TestSettingsMenuCyclesProactiveTalk(t *testing.T) {
	cfg := config.Default()
	cfg.Mode = config.ModeAI // proactive_talk is an AI-only row
	m := NewSettingsMenu(cfg)
	m.focusForTest(14) // proactive_talk is at idx 14
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

	menu.focusForTest(19) // restore_defaults
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

func TestSettingsMenuRestoreDefaultsActivates(t *testing.T) {
	m := NewSettingsMenu(config.Default())
	called := false
	m.SetRestoreDefaultsCallback(func() error { called = true; return nil })
	m.focusForTest(19) // restore_defaults
	if err := m.ToggleFocused(); err != nil {
		t.Fatalf("restore defaults: %v", err)
	}
	if !called {
		t.Fatal("restore defaults callback not invoked")
	}
}

func TestSettingsMenuAboutShowsAndDismisses(t *testing.T) {
	m := NewSettingsMenu(config.Default())
	m.SetAbout(AboutState{Name: "BMO", Version: "v1.2.3"})

	// ABOUT is the last slot, reachable below RESTORE DEFAULTS.
	if got := m.Overlay().Items[20].Code; got != "about" {
		t.Fatalf("expected about at idx 20, got %q", got)
	}
	if m.AboutActive() {
		t.Fatal("about should not be active before activation")
	}

	m.focusForTest(20)
	if err := m.ToggleFocused(); err != nil {
		t.Fatalf("activate about: %v", err)
	}
	if !m.AboutActive() {
		t.Fatal("about should be active after activation")
	}
	ov := m.Overlay()
	if ov.About == nil || ov.About.Version != "v1.2.3" {
		t.Fatalf("overlay should carry About content, got %+v", ov.About)
	}
	if len(ov.Items) != 0 {
		t.Fatalf("about overlay should not render the settings list, got %d items", len(ov.Items))
	}

	m.DismissAbout()
	if m.AboutActive() {
		t.Fatal("about should be dismissed")
	}
	if m.Overlay().About != nil {
		t.Fatal("overlay should return to the list after dismiss")
	}
}

func TestSettingsMenuAboutIgnoresArrowsActivatesOnConfirm(t *testing.T) {
	m := NewSettingsMenu(config.Default())
	m.SetAbout(AboutState{Name: "BMO"})
	m.focusForTest(20) // about

	// Arrows (Cycle) must not open the About screen.
	if err := m.Cycle(1); err != nil {
		t.Fatalf("Cycle(+1) on about: %v", err)
	}
	if err := m.Cycle(-1); err != nil {
		t.Fatalf("Cycle(-1) on about: %v", err)
	}
	if m.AboutActive() {
		t.Fatal("arrow keys must not open the About screen")
	}

	// A (ToggleFocused) opens it.
	if err := m.ToggleFocused(); err != nil {
		t.Fatalf("ToggleFocused on about: %v", err)
	}
	if !m.AboutActive() {
		t.Fatal("A button should open the About screen")
	}
}

func TestSettingsMenuRestoreIgnoresArrows(t *testing.T) {
	m := NewSettingsMenu(config.Default())
	called := false
	m.SetRestoreDefaultsCallback(func() error { called = true; return nil })
	m.focusForTest(19) // restore_defaults
	if err := m.Cycle(1); err != nil {
		t.Fatalf("Cycle on restore: %v", err)
	}
	if called {
		t.Fatal("arrow keys must not trigger Restore Defaults")
	}
	if err := m.ToggleFocused(); err != nil {
		t.Fatalf("ToggleFocused on restore: %v", err)
	}
	if !called {
		t.Fatal("A button should trigger Restore Defaults")
	}
}

func TestSettingsMenuConfirmAdvancesProvider(t *testing.T) {
	cfg := config.Default()
	cfg.Mode = config.ModeAI
	m := NewSettingsMenu(cfg)
	m.focusForTest(3) // stt_status
	before := m.Overlay().Items[3].Label
	if err := m.ToggleFocused(); err != nil {
		t.Fatalf("ToggleFocused on provider: %v", err)
	}
	// Advancing only changes the label when more than one provider exists; the
	// call must at least not error and must keep the row valid.
	if got := m.Overlay().Items[3].Code; got != "stt_status" {
		t.Fatalf("provider row changed identity: %q (before label %q)", got, before)
	}
}

func TestSettingsMenuAboutInertWithoutContent(t *testing.T) {
	m := NewSettingsMenu(config.Default()) // no SetAbout call
	m.focusForTest(20)
	if err := m.ToggleFocused(); err != nil {
		t.Fatalf("toggle about without content: %v", err)
	}
	if m.AboutActive() {
		t.Fatal("about must stay inert when no content was supplied")
	}
}

// ── AI Status Items ────────────────────────────────────────────────────────

func TestSettingsMenuAIRowsHiddenWhenIdle(t *testing.T) {
	cfg := config.Default() // Mode = "idle"
	m := NewSettingsMenu(cfg)
	items := m.Overlay().Items
	// Providers, voice, awareness, library detail, timeout and proactive talk
	// are all grouped under AI mode and hidden when it is off.
	for _, idx := range []int{3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14} {
		if !items[idx].Hidden {
			t.Errorf("items[%d] (%s) should be Hidden when mode is idle", idx, items[idx].Code)
		}
		if items[idx].Focused {
			t.Errorf("items[%d] should not be Focused when hidden", idx)
		}
	}
}

func TestSettingsMenuAIRowsIndented(t *testing.T) {
	cfg := config.Default()
	cfg.Mode = config.ModeAI
	m := NewSettingsMenu(cfg)
	items := m.Overlay().Items
	// Every AI sub-setting (3-16) is indented to nest under MODE.
	for _, idx := range []int{3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16} {
		if !items[idx].Indent {
			t.Errorf("items[%d] (%s) should be indented under AI mode", idx, items[idx].Code)
		}
	}
	// Top-level rows stay flush left.
	for _, idx := range []int{0, 2, 17, 19} {
		if items[idx].Indent {
			t.Errorf("items[%d] (%s) should not be indented", idx, items[idx].Code)
		}
	}
	// The provider/voice status rows carry a status square like every other row.
	for _, idx := range []int{3, 4, 5, 6} {
		if !items[idx].Selected {
			t.Errorf("items[%d] (%s) should show a (selected) status box", idx, items[idx].Code)
		}
	}
}

func TestSettingsMenuAIRowsVisibleWhenAI(t *testing.T) {
	cfg := config.Default()
	cfg.Mode = config.ModeAI
	m := NewSettingsMenu(cfg)
	items := m.Overlay().Items
	for _, idx := range []int{3, 4, 5, 6, 7, 12, 13, 14} {
		if items[idx].Hidden {
			t.Errorf("items[%d] (%s) should be visible when mode is ai", idx, items[idx].Code)
		}
	}
	// Voice (6) is visible but remains a read-only, non-focusable status row.
	if items[6].Focused {
		t.Errorf("voice row 6 should never be focused")
	}
}

func TestSettingsMenuAIStatusShowsModelOnly(t *testing.T) {
	cfg := config.Default()
	cfg.STT = config.ProviderSet{Active: "openai-compatible", Providers: []config.Provider{{Name: "openai-compatible", Model: "whisper-1", APIKey: "sk-s"}}}
	cfg.Chat = config.ProviderSet{Active: "openai-compatible", Providers: []config.Provider{{Name: "openai-compatible", Model: "gpt-4o-mini"}}}
	cfg.TTS = config.ProviderSet{Active: "openai-compatible", Providers: []config.Provider{{Name: "openai-compatible", Model: "tts-1", Voice: "nova", APIKey: "sk-t"}}}
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
	cfg.TTS = config.ProviderSet{Active: "openai-compatible", Providers: []config.Provider{{Name: "openai-compatible", Model: "tts-1"}}}
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
	cfg := config.Default()
	cfg.Mode = config.ModeAI // awareness rows are AI-only
	m := NewSettingsMenu(cfg)
	overlay := m.Overlay()
	if len(overlay.Items) != 21 {
		t.Fatalf("expected 21 overlay items, got %d", len(overlay.Items))
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
	if got := m.Overlay().Items[17].Code; got != "mod" {
		t.Fatalf("expected mod item at idx 17, got %q", got)
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
	if got := m.Overlay().Items[17].Label; got != "MOD: EVIL BMO" {
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

	if got := m.Overlay().Items[17].Label; got != "MOD: BMO (DEFAULT)" {
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

func (m *SettingsMenu) focusForTest(i int)     { m.focus = i }
func (m *SettingsMenu) focusIndexForTest() int { return m.focus }

func TestSettingsMenuProviderRowsFocusableWhenAI(t *testing.T) {
	cfg := config.Default()
	cfg.Mode = config.ModeAI
	cfg.STT = config.ProviderSet{Active: "a", Providers: []config.Provider{{Name: "a", Model: "whisper-1"}, {Name: "b", Model: "whisper-large"}}}
	cfg.Chat = config.ProviderSet{Providers: []config.Provider{{Name: "c", Model: "gpt-4o-mini"}}}
	cfg.TTS = config.ProviderSet{Providers: []config.Provider{{Name: "t", Model: "tts-1", Voice: "nova"}}}
	m := NewSettingsMenu(cfg)

	reachable := map[int]bool{}
	for i := 0; i < 30; i++ {
		reachable[m.focusIndexForTest()] = true
		m.Move(1)
	}
	for _, idx := range []int{3, 4, 5} {
		if !reachable[idx] {
			t.Errorf("row %d not focusable in AI mode", idx)
		}
	}
	if reachable[6] {
		t.Error("voice row 6 should remain non-focusable")
	}
}

func TestSettingsMenuCycleChangesActiveProvider(t *testing.T) {
	cfg := config.Default()
	cfg.Mode = config.ModeAI
	cfg.STT = config.ProviderSet{Active: "a", Providers: []config.Provider{{Name: "a", Model: "whisper-1"}, {Name: "b", Model: "whisper-large"}}}
	cfg.Chat = config.ProviderSet{Providers: []config.Provider{{Name: "c", Model: "gpt-4o-mini"}}}
	cfg.TTS = config.ProviderSet{Providers: []config.Provider{{Name: "t", Model: "tts-1"}}}
	m := NewSettingsMenu(cfg)
	m.focusForTest(3)

	if err := m.Cycle(1); err != nil {
		t.Fatalf("Cycle(1) error = %v", err)
	}
	if got := m.Config().STT.Active; got != "b" {
		t.Fatalf("after Cycle(1) STT.Active = %q, want b", got)
	}
	if err := m.Cycle(-1); err != nil {
		t.Fatalf("Cycle(-1) error = %v", err)
	}
	if got := m.Config().STT.Active; got != "a" {
		t.Fatalf("after Cycle(-1) STT.Active = %q, want a", got)
	}
	if got := m.Overlay().Items[3].Label; got != "STT: whisper-1" {
		t.Fatalf("stt label = %q, want STT: whisper-1", got)
	}
	m.focusForTest(3)
	_ = m.Cycle(1)
	if got := m.Overlay().Items[3].Label; got != "STT: whisper-large" {
		t.Fatalf("stt label after cycle = %q, want STT: whisper-large", got)
	}
}

func TestSettingsMenuCycleNonProviderDelegatesForward(t *testing.T) {
	cfg := config.Default()
	m := NewSettingsMenu(cfg)
	m.focusForTest(13) // request_timeout
	before := m.Config().RequestTimeout
	if err := m.Cycle(-1); err != nil { // negative still advances forward
		t.Fatalf("Cycle(-1) error = %v", err)
	}
	if m.Config().RequestTimeout == before {
		t.Fatalf("Cycle on timeout row did not advance (still %d)", before)
	}
}

func TestSettingsMenuProviderRowShowsFocusWhenAI(t *testing.T) {
	cfg := config.Default()
	cfg.Mode = config.ModeAI
	cfg.STT = config.ProviderSet{Active: "a", Providers: []config.Provider{{Name: "a", Model: "whisper-1"}, {Name: "b", Model: "whisper-large"}}}
	cfg.Chat = config.ProviderSet{Providers: []config.Provider{{Name: "c", Model: "gpt-4o-mini"}}}
	cfg.TTS = config.ProviderSet{Providers: []config.Provider{{Name: "t", Model: "tts-1", Voice: "nova"}}}
	m := NewSettingsMenu(cfg)

	for _, idx := range []int{3, 4, 5} {
		m.focusForTest(idx)
		items := m.Overlay().Items
		if !items[idx].Focused {
			t.Errorf("provider row %d (%s) not marked Focused when selected in AI mode", idx, items[idx].Code)
		}
		// Other provider rows must not also report focus.
		for _, other := range []int{3, 4, 5} {
			if other != idx && items[other].Focused {
				t.Errorf("row %d focused while focus is on %d", other, idx)
			}
		}
		// Voice row (6) must never report focus.
		if items[6].Focused {
			t.Errorf("voice row 6 should never be focused")
		}
	}
}

func TestSettingsWakeWordToggle(t *testing.T) {
	cfg := config.Default()
	cfg.Mode = config.ModeAI
	m := NewSettingsMenu(cfg)
	m.focusForTest(15) // wake_word
	if got := m.Overlay().Items[15].Code; got != "wake_word" {
		t.Fatalf("expected wake_word at idx 15, got %q", got)
	}
	if err := m.ToggleFocused(); err != nil {
		t.Fatalf("toggle: %v", err)
	}
	if !m.Config().WakeWordEnabled {
		t.Fatal("wake word not enabled after toggle")
	}
	if m.Config().InputTrigger != config.InputTriggerWakeWord {
		t.Fatalf("enabling wake word should set trigger, got %q", m.Config().InputTrigger)
	}
	// Disabling reverts the trigger to PTT.
	if err := m.ToggleFocused(); err != nil {
		t.Fatalf("toggle back: %v", err)
	}
	if m.Config().WakeWordEnabled {
		t.Fatal("wake word still enabled after second toggle")
	}
	if m.Config().InputTrigger != config.InputTriggerPTT {
		t.Fatalf("disabling wake word should restore PTT, got %q", m.Config().InputTrigger)
	}
}

func TestSettingsContinuedConvoHiddenUntilWakeWord(t *testing.T) {
	cfg := config.Default()
	cfg.Mode = config.ModeAI
	m := NewSettingsMenu(cfg)
	if !m.Overlay().Items[16].Hidden {
		t.Fatal("continued_convo should be hidden when wake word is off")
	}
	m.focusForTest(15)
	_ = m.ToggleFocused() // enable wake word
	if m.Overlay().Items[16].Hidden {
		t.Fatal("continued_convo should be visible once wake word is on")
	}
}

func TestSettingsContinuedConvoCycles(t *testing.T) {
	cfg := config.Default()
	cfg.Mode = config.ModeAI
	cfg.WakeWordEnabled = true
	cfg.ContinuedConversation = config.ContinuedConvoOff
	m := NewSettingsMenu(cfg)
	m.focusForTest(16) // continued_convo
	want := []string{config.ContinuedConvoShort, config.ContinuedConvoLong, config.ContinuedConvoOff}
	for _, v := range want {
		if err := m.Cycle(1); err != nil {
			t.Fatalf("cycle: %v", err)
		}
		if got := m.Config().ContinuedConversation; got != v {
			t.Fatalf("continued conversation = %q, want %q", got, v)
		}
	}
}
