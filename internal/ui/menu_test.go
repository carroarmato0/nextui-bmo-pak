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
	var southButton *OverlayItem
	for i := range overlay.Items {
		switch overlay.Items[i].Code {
		case "BTN_SOUTH":
			southButton = &overlay.Items[i]
		}
	}
	if southButton == nil || !southButton.Selected {
		t.Fatalf("south button entry = %+v, want selected", southButton)
	}
}

func TestSetupMenuToggleAndSave(t *testing.T) {
	menu := NewSetupMenu(config.Default())
	// default is [BTN_SOUTH], toggle to [BTN_SOUTH, BTN_TL]
	menu.Move(5)
	if err := menu.ToggleFocused(); err != nil {
		t.Fatalf("ToggleFocused() error = %v", err)
	}
	got := menu.Config().PTTButtons
	if len(got) != 2 || got[0] != "BTN_SOUTH" || got[1] != "BTN_TL" {
		t.Fatalf("PTTButtons = %+v, want [BTN_SOUTH BTN_TL]", got)
	}

	saved, err := menu.Save()
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if !saved.SetupComplete {
		t.Fatal("Save() should mark setup complete")
	}
	if len(saved.PTTButtons) != 2 || saved.PTTButtons[0] != "BTN_SOUTH" || saved.PTTButtons[1] != "BTN_TL" {
		t.Fatalf("saved PTTButtons = %+v, want [BTN_SOUTH BTN_TL]", saved.PTTButtons)
	}
}

func TestSettingsMenuUsesSamePTTChoices(t *testing.T) {
	cfg := config.Default()
	cfg.PTTButtons = []string{"BTN_THUMBL"}
	menu := NewPTTMenu(cfg)
	overlay := menu.Overlay()

	if overlay.Title != "SETTINGS" {
		t.Fatalf("Overlay().Title = %q, want SETTINGS", overlay.Title)
	}
	if overlay.Items[12].Label != "LEFT STICK" || !overlay.Items[12].Selected {
		t.Fatalf("LEFT STICK entry = %+v", overlay.Items[12])
	}
}
