package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	want := Default()
	want.Mode = ModeAI
	want.InputTrigger = InputTriggerWakeWord
	want.LogLevel = "debug"
	want.STT = Provider{Name: "openai-compatible", Model: "whisper-1", BaseURL: "https://example.invalid", APIKey: "secret-stt"}
	want.Chat = Provider{Name: "openai-compatible", Model: "gpt-4o-mini", APIKey: "secret-chat"}
	want.TTS = Provider{Name: "openai-compatible", Model: "tts-1", Voice: "alloy", APIKey: "secret-tts"}
	want.PTTButtons = []string{"BTN_TL2", "BTN_TR2"}
	want.LogSystemPrompt = true

	if err := Save(path, want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got.Mode != want.Mode || got.InputTrigger != want.InputTrigger || got.LogLevel != want.LogLevel || got.Personality != want.Personality {
		t.Fatalf("round-trip mismatch: got %+v want %+v", got, want)
	}
	if got.STT.APIKey != want.STT.APIKey || got.Chat.APIKey != want.Chat.APIKey || got.TTS.APIKey != want.TTS.APIKey || got.TTS.Voice != want.TTS.Voice || len(got.PTTButtons) != len(want.PTTButtons) || got.PTTButtons[0] != want.PTTButtons[0] || got.PTTButtons[1] != want.PTTButtons[1] {
		t.Fatalf("provider fields lost: got %+v want %+v", got, want)
	}
	if got.LogSystemPrompt != want.LogSystemPrompt {
		t.Fatalf("LogSystemPrompt lost: got %v want %v", got.LogSystemPrompt, want.LogSystemPrompt)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestLoadMissingReturnsDefaultAndSentinel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.json")

	got, err := Load(path)
	if err != ErrNotFound {
		t.Fatalf("Load() error = %v, want ErrNotFound", err)
	}
	if got.Mode != ModeIdle || got.LogLevel != DefaultLogLevel || got.Personality != DefaultPersonality || got.InputTrigger != InputTriggerPTT {
		t.Fatalf("unexpected defaults: %+v", got)
	}
}

func TestSaveCreatesParents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "config.json")

	if err := Save(path, Default()); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config not written: %v", err)
	}
}

func TestRedactedRemovesSecrets(t *testing.T) {
	cfg := Default()
	cfg.STT.APIKey = "secret-stt"
	cfg.Chat.APIKey = "secret-chat"
	cfg.TTS.APIKey = "secret-tts"

	redacted := cfg.Redacted()
	if redacted.STT.APIKey != "" || redacted.Chat.APIKey != "" || redacted.TTS.APIKey != "" {
		t.Fatalf("Redacted() kept secrets: %+v", redacted)
	}
}

func TestValidateAllowsIdleWithoutProviders(t *testing.T) {
	cfg := Default()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsAIWithoutProviders(t *testing.T) {
	cfg := Default()
	cfg.Mode = ModeAI
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
}

func TestDefaultPTTButtonIsA(t *testing.T) {
	cfg := Default()
	if len(cfg.PTTButtons) != 1 || cfg.PTTButtons[0] != "BTN_EAST" {
		t.Fatalf("expected PTTButtons=[BTN_SOUTH], got %v", cfg.PTTButtons)
	}
}

func TestLoadIgnoresRemovedPromptKeys(t *testing.T) {
	// Configs written before prompts moved to persona.txt/voice.txt may still
	// carry system_prompt and tts.instructions; they must load cleanly.
	path := filepath.Join(t.TempDir(), "config.json")
	legacy := `{"version":1,"mode":"ai","system_prompt":"old persona","tts":{"name":"openai-compatible","model":"gpt-4o-mini-tts","voice":"nova","instructions":"old instructions"},"log_level":"info","personality":"bmo","reduced_motion":false}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.TTS.Model != "gpt-4o-mini-tts" || cfg.TTS.Voice != "nova" {
		t.Fatalf("legacy config fields lost: %#v", cfg.TTS)
	}
}

func TestDefaultSetupComplete(t *testing.T) {
	cfg := Default()
	if !cfg.SetupComplete {
		t.Fatal("expected SetupComplete=true in default config")
	}
}

func TestDefaultDeviceContextAllEnabled(t *testing.T) {
	cfg := Default()
	dc := cfg.DeviceContext
	if !dc.Library || !dc.Saves || !dc.PlayLog || !dc.System || !dc.Achievements {
		t.Fatalf("expected all device context categories enabled by default, got %+v", dc)
	}
	if cfg.ProactiveTalk != ProactiveOff {
		t.Fatalf("expected proactive talk off by default, got %q", cfg.ProactiveTalk)
	}
}

func TestLoadConfigWithoutDeviceContextDefaultsEnabled(t *testing.T) {
	// Configs written before this feature have no device_context key; the
	// Load-over-Default merge must leave every category enabled.
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"mode":"idle","log_level":"info","personality":"bmo","stt":{},"chat":{},"tts":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	dc := cfg.DeviceContext
	if !dc.Library || !dc.Saves || !dc.PlayLog || !dc.System || !dc.Achievements {
		t.Fatalf("expected legacy config to default all categories on, got %+v", dc)
	}
}

func TestNormalizeProactiveTalk(t *testing.T) {
	cfg := Config{ProactiveTalk: "  CHATTY "}
	cfg.Normalize()
	if cfg.ProactiveTalk != ProactiveChatty {
		t.Fatalf("expected normalized chatty, got %q", cfg.ProactiveTalk)
	}
	cfg = Config{}
	cfg.Normalize()
	if cfg.ProactiveTalk != ProactiveOff {
		t.Fatalf("expected empty level normalized to off, got %q", cfg.ProactiveTalk)
	}
}

func TestValidateRejectsUnknownProactiveTalk(t *testing.T) {
	cfg := Default()
	cfg.ProactiveTalk = "constantly"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for unknown proactive talk level")
	}
}

func TestProactiveInterval(t *testing.T) {
	cases := map[string]time.Duration{
		ProactiveOff:        0,
		ProactiveChatty:     7 * time.Minute,
		ProactiveRegular:    30 * time.Minute,
		ProactiveOccasional: time.Hour,
		ProactiveRare:       3 * time.Hour,
		"bogus":             0,
		"":                  0,
	}
	for level, want := range cases {
		if got := ProactiveInterval(level); got != want {
			t.Errorf("ProactiveInterval(%q) = %v, want %v", level, got, want)
		}
	}
}

func TestRequestTimeoutDefaultsTo15(t *testing.T) {
	cfg := Default()
	cfg.Normalize()
	if cfg.RequestTimeout != 15 {
		t.Fatalf("want 15, got %d", cfg.RequestTimeout)
	}
}

func TestRequestTimeoutClampedWhenTooLow(t *testing.T) {
	cfg := Config{RequestTimeout: 5}
	cfg.Normalize()
	if cfg.RequestTimeout != 15 {
		t.Fatalf("want 15 (clamped), got %d", cfg.RequestTimeout)
	}
}

func TestRequestTimeoutClampedWhenTooHigh(t *testing.T) {
	cfg := Config{RequestTimeout: 999}
	cfg.Normalize()
	if cfg.RequestTimeout != 60 {
		t.Fatalf("want 60 (clamped), got %d", cfg.RequestTimeout)
	}
}

func TestSupportedRequestTimeoutsContains15And60(t *testing.T) {
	vals := SupportedRequestTimeouts()
	has15, has60 := false, false
	for _, v := range vals {
		if v == 15 {
			has15 = true
		}
		if v == 60 {
			has60 = true
		}
	}
	if !has15 {
		t.Fatal("SupportedRequestTimeouts missing 15")
	}
	if !has60 {
		t.Fatal("SupportedRequestTimeouts missing 60")
	}
}

func TestActiveModRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := Default()
	cfg.ActiveMod = "evil"
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.ActiveMod != "evil" {
		t.Fatalf("ActiveMod = %q, want evil", got.ActiveMod)
	}
}

func TestActiveModNormalizesWhitespace(t *testing.T) {
	cfg := Default()
	cfg.ActiveMod = "  evil  "
	cfg.Normalize()
	if cfg.ActiveMod != "evil" {
		t.Fatalf("ActiveMod = %q, want trimmed 'evil'", cfg.ActiveMod)
	}
}
