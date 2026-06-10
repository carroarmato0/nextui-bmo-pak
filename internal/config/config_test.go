package config

import (
	"os"
	"path/filepath"
	"testing"
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
	want.SystemPrompt = "hello"

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
