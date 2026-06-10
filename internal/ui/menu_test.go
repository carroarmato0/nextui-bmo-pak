package ui

import (
	"testing"

	"github.com/carroarmato0/nextui-bmo/internal/config"
)

func TestSetupMenuBuildsOverlayWithPTTChoices(t *testing.T) {
	menu := NewSetupMenu(config.Default())
	overlay := menu.Overlay()

	if overlay.Title != "SETUP" {
		t.Fatalf("Overlay().Title = %q, want SETUP", overlay.Title)
	}
	if len(overlay.Items) == 0 {
		t.Fatal("Overlay().Items is empty")
	}
	var leftTrigger, rightTrigger *OverlayItem
	for i := range overlay.Items {
		switch overlay.Items[i].Code {
		case "BTN_TL":
			leftTrigger = &overlay.Items[i]
		case "BTN_TR":
			rightTrigger = &overlay.Items[i]
		}
	}
	if leftTrigger == nil || !leftTrigger.Selected {
		t.Fatalf("left trigger entry = %+v, want selected", leftTrigger)
	}
	if rightTrigger == nil || !rightTrigger.Selected {
		t.Fatalf("right trigger entry = %+v, want selected", rightTrigger)
	}
}

func TestSetupMenuToggleAndSave(t *testing.T) {
	menu := NewSetupMenu(config.Default())
	menu.Move(6)
	if err := menu.ToggleFocused(); err != nil {
		t.Fatalf("ToggleFocused() error = %v", err)
	}
	got := menu.Config().PTTButtons
	if len(got) != 1 || got[0] != "BTN_TL" {
		t.Fatalf("PTTButtons = %+v, want [BTN_TL]", got)
	}

	saved, err := menu.Save()
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if !saved.SetupComplete {
		t.Fatal("Save() should mark setup complete")
	}
	if saved.PTTButtons[0] != "BTN_TL" {
		t.Fatalf("saved PTTButtons = %+v", saved.PTTButtons)
	}
}

func TestSettingsMenuUsesSamePTTChoices(t *testing.T) {
	cfg := config.Default()
	cfg.PTTButtons = []string{"BTN_THUMBL"}
	menu := NewSettingsMenu(cfg)
	overlay := menu.Overlay()

	if overlay.Title != "SETTINGS" {
		t.Fatalf("Overlay().Title = %q, want SETTINGS", overlay.Title)
	}
	if overlay.Items[12].Label != "LEFT STICK" || !overlay.Items[12].Selected {
		t.Fatalf("LEFT STICK entry = %+v", overlay.Items[12])
	}
}
