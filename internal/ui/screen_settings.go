package ui

import (
	"fmt"

	"github.com/carroarmato0/nextui-bmo/internal/config"
)

type SettingsScreen struct {
	cfg config.Config
}

func NewSettingsScreen(cfg config.Config) *SettingsScreen {
	cfg.Normalize()
	return &SettingsScreen{cfg: cfg}
}

func (s *SettingsScreen) ReopenSetup() ScreenID {
	return ScreenSetup
}

func (s *SettingsScreen) PTTButtons() []string {
	if s == nil {
		return config.DefaultPTTButtons()
	}
	return append([]string(nil), s.cfg.PTTButtons...)
}

func (s *SettingsScreen) SetPTTButtons(buttons []string) error {
	if s == nil {
		return fmt.Errorf("nil settings screen")
	}
	if err := config.ValidatePTTButtons(buttons); err != nil {
		return err
	}
	s.cfg.PTTButtons = config.NormalizePTTButtons(buttons)
	return nil
}

func (s *SettingsScreen) ResetPTTButtons() {
	if s == nil {
		return
	}
	s.cfg.PTTButtons = config.DefaultPTTButtons()
}

func (s *SettingsScreen) Config() config.Config {
	if s == nil {
		return config.Default()
	}
	return s.cfg
}
