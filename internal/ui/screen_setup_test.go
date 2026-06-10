package ui

import (
	"testing"

	"github.com/carroarmato0/nextui-bmo/internal/config"
)

func TestInitialScreenForMissingFirstRunConfigIsSetup(t *testing.T) {
	cfg := config.Default()
	cfg.SetupComplete = false

	flow := NewSetupFlow(cfg)
	if got := flow.InitialScreen(); got != ScreenSetup {
		t.Fatalf("InitialScreen() = %q, want setup", got)
	}
}

func TestInitialScreenForCompletedIdleOnlyConfigIsMain(t *testing.T) {
	cfg := config.Default()
	cfg.SetupComplete = true
	cfg.Mode = config.ModeIdle

	flow := NewSetupFlow(cfg)
	if got := flow.InitialScreen(); got != ScreenMain {
		t.Fatalf("InitialScreen() = %q, want main", got)
	}
}

func TestIdleOnlySelectionCanSaveWithoutProviders(t *testing.T) {
	cfg := config.Default()
	flow := NewSetupFlow(cfg)

	screen := flow.SetupScreen()
	screen.SelectIdleOnly()
	if err := screen.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	saved := screen.Config()
	if saved.Mode != config.ModeIdle {
		t.Fatalf("saved mode = %q, want idle", saved.Mode)
	}
	if !saved.SetupComplete {
		t.Fatal("saved config should mark setup complete")
	}
	if err := saved.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestAISetupRequiresValidProvidersBeforeSave(t *testing.T) {
	flow := NewSetupFlow(config.Default())
	screen := flow.SetupScreen()
	screen.SelectAIMode()

	if err := screen.Save(); err == nil {
		t.Fatal("Save() error = nil, want validation error")
	}

	screen.SetProvider("stt", config.Provider{Name: "openai-compatible", Model: "whisper-1"})
	screen.SetProvider("chat", config.Provider{Name: "openai-compatible", Model: "gpt-4o-mini"})
	screen.SetProvider("tts", config.Provider{Name: "openai-compatible", Model: "tts-1"})
	if err := screen.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	saved := screen.Config()
	if !saved.UsesAI() {
		t.Fatalf("saved mode = %q, want ai", saved.Mode)
	}
	if err := saved.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestSetupScreenPTTButtonsCanBeChangedAndSaved(t *testing.T) {
	flow := NewSetupFlow(config.Default())
	screen := flow.SetupScreen()

	if err := screen.SetPTTButtons([]string{"btn_tl2", "BTN_TR2", "BTN_TR2"}); err != nil {
		t.Fatalf("SetPTTButtons() error = %v", err)
	}
	if got := screen.PTTButtons(); len(got) != 2 || got[0] != "BTN_TL2" || got[1] != "BTN_TR2" {
		t.Fatalf("PTTButtons() = %+v, want [BTN_TL2 BTN_TR2]", got)
	}
	if err := screen.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if got := screen.Config().PTTButtons; len(got) != 2 || got[0] != "BTN_TL2" || got[1] != "BTN_TR2" {
		t.Fatalf("saved PTTButtons = %+v, want [BTN_TL2 BTN_TR2]", got)
	}
}

func TestSetupScreenRejectsUnknownPTTButton(t *testing.T) {
	flow := NewSetupFlow(config.Default())
	screen := flow.SetupScreen()

	if err := screen.SetPTTButtons([]string{"BTN_TL", "BTN_UNKNOWN"}); err == nil {
		t.Fatal("SetPTTButtons() error = nil, want validation error")
	}
}

func TestSettingsCanReopenSetup(t *testing.T) {
	cfg := config.Default()
	cfg.SetupComplete = true
	settings := NewSettingsScreen(cfg)

	if got := settings.ReopenSetup(); got != ScreenSetup {
		t.Fatalf("ReopenSetup() = %q, want setup", got)
	}
}

func TestSettingsScreenPTTButtonsMirrorSetupScreen(t *testing.T) {
	cfg := config.Default()
	settings := NewSettingsScreen(cfg)

	if err := settings.SetPTTButtons([]string{"BTN_THUMBL"}); err != nil {
		t.Fatalf("SetPTTButtons() error = %v", err)
	}
	if got := settings.PTTButtons(); len(got) != 1 || got[0] != "BTN_THUMBL" {
		t.Fatalf("PTTButtons() = %+v, want [BTN_THUMBL]", got)
	}
	if err := settings.Config().Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}
