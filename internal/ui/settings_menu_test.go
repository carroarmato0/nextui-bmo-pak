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
	if got := menu.Overlay().Items[0].Code; got != "mode" {
		t.Fatalf("first item code = %q, want mode", got)
	}

	if err := menu.ToggleFocused(); err != nil {
		t.Fatalf("ToggleFocused() error = %v", err)
	}
	if got := menu.Config().Mode; got != config.ModeAI {
		t.Fatalf("Mode = %q, want ai", got)
	}

	menu.SetProvider("stt", config.Provider{Name: "openai-compatible", Model: "whisper-1"})
	menu.SetProvider("chat", config.Provider{Name: "openai-compatible", Model: "gpt-4o-mini"})
	menu.SetProvider("tts", config.Provider{Name: "openai-compatible", Model: "tts-1", Voice: "alloy"})

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
