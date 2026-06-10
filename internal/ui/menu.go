package ui

import (
	"fmt"
	"strings"

	"github.com/carroarmato0/nextui-bmo/internal/config"
	"github.com/carroarmato0/nextui-bmo/internal/input"
)

type OverlayItem struct {
	Code     string
	Label    string
	Selected bool
	Focused  bool
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
	Save() (config.Config, error)
	Overlay() OverlayState
}

type PTTMenu struct {
	title string
	cfg   config.Config
	focus int
}

func NewSetupMenu(cfg config.Config) *PTTMenu {
	cfg.Normalize()
	return &PTTMenu{title: "SETUP", cfg: cfg}
}

func NewSettingsMenu(cfg config.Config) *PTTMenu {
	cfg.Normalize()
	return &PTTMenu{title: "SETTINGS", cfg: cfg}
}

func (m *PTTMenu) Title() string {
	if m == nil || strings.TrimSpace(m.title) == "" {
		return "SETUP"
	}
	return strings.ToUpper(strings.TrimSpace(m.title))
}

func (m *PTTMenu) Config() config.Config {
	if m == nil {
		return config.Default()
	}
	return m.cfg
}

func (m *PTTMenu) Move(delta int) {
	if m == nil {
		return
	}
	count := len(config.SupportedPTTButtons())
	if count == 0 {
		m.focus = 0
		return
	}
	m.focus = (m.focus + delta) % count
	if m.focus < 0 {
		m.focus += count
	}
}

func (m *PTTMenu) ToggleFocused() error {
	if m == nil {
		return fmt.Errorf("nil menu")
	}
	buttons := config.SupportedPTTButtons()
	if len(buttons) == 0 {
		return fmt.Errorf("no ptt buttons available")
	}
	if m.focus < 0 || m.focus >= len(buttons) {
		return fmt.Errorf("focus out of range")
	}
	code := buttons[m.focus]
	next, err := togglePTTButtonState(m.cfg.PTTButtons, code)
	if err != nil {
		return err
	}
	m.cfg.PTTButtons = next
	return nil
}

func (m *PTTMenu) Save() (config.Config, error) {
	if m == nil {
		return config.Config{}, fmt.Errorf("nil menu")
	}
	cfg := m.cfg
	cfg.PTTButtons = config.NormalizePTTButtons(cfg.PTTButtons)
	cfg.SetupComplete = true
	if err := cfg.Validate(); err != nil {
		return config.Config{}, err
	}
	m.cfg = cfg
	return cfg, nil
}

func (m *PTTMenu) Overlay() OverlayState {
	if m == nil {
		return OverlayState{}
	}
	buttons := config.SupportedPTTButtons()
	items := make([]OverlayItem, 0, len(buttons))
	selected := make(map[string]struct{}, len(m.cfg.PTTButtons))
	for _, b := range m.cfg.PTTButtons {
		selected[input.NormalizeButtonName(b)] = struct{}{}
	}
	for idx, code := range buttons {
		name := input.ButtonLabel(code)
		items = append(items, OverlayItem{
			Code:     code,
			Label:    strings.ToUpper(name),
			Selected: func() bool { _, ok := selected[code]; return ok }(),
			Focused:  idx == m.focus,
		})
	}
	return OverlayState{
		Visible: true,
		Title:   m.title,
		Subtitle: []string{
			"ENTER TO TOGGLE",
			"START TO SAVE",
		},
		Items:  items,
		Footer: joinPTTButtonLabels(m.cfg.PTTButtons),
	}
}
