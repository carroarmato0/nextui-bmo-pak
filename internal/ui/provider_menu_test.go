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
	menu.InsertText("secret-stt")
	if err := menu.SubmitEdit(); err != nil {
		t.Fatalf("SubmitEdit() error = %v", err)
	}
	menu.Move(1)
	if err := menu.ToggleFocused(); err != nil {
		t.Fatalf("ToggleFocused() error = %v", err)
	}
	menu.Move(1)
	if err := menu.ToggleFocused(); err != nil {
		t.Fatalf("ToggleFocused() error = %v", err)
	}
	menu.InsertText("secret-chat")
	if err := menu.SubmitEdit(); err != nil {
		t.Fatalf("SubmitEdit() error = %v", err)
	}
	menu.Move(1)
	if err := menu.ToggleFocused(); err != nil {
		t.Fatalf("ToggleFocused() error = %v", err)
	}
	menu.Move(1)
	if err := menu.ToggleFocused(); err != nil {
		t.Fatalf("ToggleFocused() error = %v", err)
	}
	menu.InsertText("secret-tts")
	if err := menu.SubmitEdit(); err != nil {
		t.Fatalf("SubmitEdit() error = %v", err)
	}
	if got := menu.Overlay().Items[2].Label; got == "STT: KEY MISSING" {
		t.Fatal("expected STT key label to reflect API key presence")
	}

	saved, err := menu.Save()
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if !saved.SetupComplete {
		t.Fatal("Save() should mark setup complete")
	}
}

func TestProviderMenuAPIKeyEditingFlow(t *testing.T) {
	menu := NewProviderMenu(config.Default())
	menu.Move(2)
	if got := menu.Overlay().Items[2].Code; got != "stt_key" {
		t.Fatalf("focus item code = %q, want stt_key", got)
	}
	if err := menu.ToggleFocused(); err != nil {
		t.Fatalf("ToggleFocused() enter edit error = %v", err)
	}
	if !menu.IsEditing() {
		t.Fatal("expected menu to enter edit mode")
	}
	if menu.EditingKind() != "stt" {
		t.Fatalf("EditingKind() = %q, want stt", menu.EditingKind())
	}
	menu.InsertText("sk-test")
	menu.Backspace()
	menu.InsertText("3")
	if got := menu.EditBuffer(); got != "sk-tes3" {
		t.Fatalf("EditBuffer() = %q, want sk-tes3", got)
	}
	if err := menu.SubmitEdit(); err != nil {
		t.Fatalf("SubmitEdit() error = %v", err)
	}
	if menu.IsEditing() {
		t.Fatal("expected edit mode to end after submit")
	}
	if got := menu.Config().STT.APIKey; got != "sk-tes3" {
		t.Fatalf("STT.APIKey = %q, want sk-tes3", got)
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
