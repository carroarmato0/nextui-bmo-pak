package ui

import (
	"github.com/carroarmato0/nextui-bmo/internal/config"
)

type OverlayItem struct {
	Code     string
	Label    string
	Selected bool
	Focused  bool
	Disabled bool
	Hidden   bool
}

type OverlayState struct {
	Visible  bool
	Title    string
	Subtitle []string
	Items    []OverlayItem
	Footer   string
}

type Menu interface {
	Title() string
	Move(delta int)
	ToggleFocused() error
	Cycle(delta int) error
	Save() (config.Config, error)
	Overlay() OverlayState
}
