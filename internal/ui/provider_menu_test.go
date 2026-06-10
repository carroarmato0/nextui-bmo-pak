package ui

import (
	"testing"

	"github.com/carroarmato0/nextui-bmo/internal/config"
)

func TestProviderMenuCyclesProfilesAndSaves(t *testing.T) {
	menu := NewProviderMenu(config.Default())
	overlay := menu.Overlay()
	if overlay.Title != "AI SETUP" {
		t.Fatalf("Overlay().Title = %q, want AI SETUP", overlay.Title)
	}
	if got := overlay.Items[0].Label; got != "MODE: AI" {
		t.Fatalf("mode label = %q", got)
	}

	if err := menu.ToggleFocused(); err != nil {
		t.Fatalf("ToggleFocused() error = %v", err)
	}
	if got := menu.Config().Mode; got != config.ModeIdle {
		t.Fatalf("Mode = %q, want idle after toggle", got)
	}
	if err := menu.ToggleFocused(); err != nil {
		t.Fatalf("ToggleFocused() error = %v", err)
	}
	if got := menu.Config().Mode; got != config.ModeAI {
		t.Fatalf("Mode = %q, want ai after second toggle", got)
	}

	menu.Move(1)
	if err := menu.ToggleFocused(); err != nil {
		t.Fatalf("ToggleFocused() error = %v", err)
	}
	if got := menu.Config().STT.Name; got != "openai-compatible" {
		t.Fatalf("STT.Name = %q", got)
	}
	if got := menu.Config().STT.Model; got != "whisper-1" {
		t.Fatalf("STT.Model = %q", got)
	}


	menu.Move(1)
	if err := menu.ToggleFocused(); err != nil {
		t.Fatalf("ToggleFocused() error = %v", err)
	}
	menu.Move(1)
	if err := menu.ToggleFocused(); err != nil {
		t.Fatalf("ToggleFocused() error = %v", err)
	}
	if err := menu.SetAPIKey("stt", "secret"); err != nil {
		t.Fatalf("SetAPIKey() error = %v", err)
	}
	if err := menu.SetAPIKey("chat", "secret-chat"); err != nil {
		t.Fatalf("SetAPIKey() error = %v", err)
	}
	if err := menu.SetAPIKey("tts", "secret-tts"); err != nil {
		t.Fatalf("SetAPIKey() error = %v", err)
	}

	if got := menu.Overlay().Items[1].Label; got == "STT: NOT SET" {
		t.Fatal("expected STT label to reflect API key presence")
	}

	saved, err := menu.Save()
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if !saved.SetupComplete {
		t.Fatal("Save() should mark setup complete")
	}
}

func TestProviderSummaryReportsMissingAndSetKeys(t *testing.T) {
	screen := NewSetupScreen(config.Default())
	if got := screen.ProviderSummary("stt"); got != "STT: NOT SET" {
		t.Fatalf("ProviderSummary() = %q, want missing", got)
	}
	screen.SetProvider("stt", config.Provider{Name: "openai-compatible", Model: "whisper-1", APIKey: "secret"})
	if got := screen.ProviderSummary("stt"); got != "STT: OPENAI-COMPATIBLE • whisper-1 • KEY SET" {
		t.Fatalf("ProviderSummary() = %q", got)
	}
}
