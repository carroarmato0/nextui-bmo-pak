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
	// Spacer marks a blank, non-selectable separator row. It is rendered as
	// empty vertical space (no box, no label) and is skipped during navigation.
	Spacer bool
	// Indent shifts the row right by one status-box width plus margin, used to
	// nest sub-settings visually under the parent setting they belong to.
	Indent bool
}

type OverlayState struct {
	Visible    bool
	Title      string
	Subtitle   []string
	Items      []OverlayItem
	Footer     string
	FocusIndex int
	// About, when non-nil, replaces the list view with the About screen. The
	// renderer draws this instead of Items.
	About *AboutState
}

// AboutState is the content of the About screen: a centred information panel
// with a scannable QR code linking to the project. All fields are static for a
// given build (Version is injected at build time, QR is derived from URL), so
// it is built once and reused.
type AboutState struct {
	Name        string
	Description []string // body lines, rendered centred
	Version     string
	Attribution []string // credit lines, rendered centred
	URL         string   // shown beneath the QR; also the QR payload
	QR          [][]bool // QR module matrix (true = dark), includes quiet zone
}

type Menu interface {
	Title() string
	Move(delta int)
	ToggleFocused() error
	Cycle(delta int) error
	Save() (config.Config, error)
	Overlay() OverlayState
}
