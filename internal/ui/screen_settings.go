package ui

import (
	"fmt"
	"strings"

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

func (s *SettingsScreen) PTTButtonChoices() []PTTButtonChoice {
	if s == nil {
		return buildPTTButtonChoices(config.DefaultPTTButtons())
	}
	return buildPTTButtonChoices(s.cfg.PTTButtons)
}

func (s *SettingsScreen) PTTButtonSummary() string {
	if s == nil {
		return joinPTTButtonLabels(config.DefaultPTTButtons())
	}
	return joinPTTButtonLabels(s.cfg.PTTButtons)
}

func (s *SettingsScreen) EnablePTTButton(code string) error {
	if s == nil {
		return fmt.Errorf("nil settings screen")
	}
	next, err := setPTTButtonState(s.cfg.PTTButtons, code, true)
	if err != nil {
		return err
	}
	s.cfg.PTTButtons = next
	return nil
}

func (s *SettingsScreen) DisablePTTButton(code string) error {
	if s == nil {
		return fmt.Errorf("nil settings screen")
	}
	next, err := setPTTButtonState(s.cfg.PTTButtons, code, false)
	if err != nil {
		return err
	}
	s.cfg.PTTButtons = next
	return nil
}

func (s *SettingsScreen) TogglePTTButton(code string) error {
	if s == nil {
		return fmt.Errorf("nil settings screen")
	}
	next, err := togglePTTButtonState(s.cfg.PTTButtons, code)
	if err != nil {
		return err
	}
	s.cfg.PTTButtons = next
	return nil
}

func (s *SettingsScreen) SetAPIKey(kind, key string) error {
	if s == nil {
		return fmt.Errorf("nil settings screen")
	}
	key = strings.TrimSpace(key)
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "stt":
		s.cfg.STT.APIKey = key
	case "chat":
		s.cfg.Chat.APIKey = key
	case "tts":
		s.cfg.TTS.APIKey = key
	default:
		return fmt.Errorf("unknown provider kind %q", kind)
	}
	return nil
}

func (s *SettingsScreen) ProviderSummary(kind string) string {
	if s == nil {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "stt":
		return providerSummaryLabel("STT", s.cfg.STT)
	case "chat":
		return providerSummaryLabel("CHAT", s.cfg.Chat)
	case "tts":
		return providerSummaryLabel("TTS", s.cfg.TTS)
	default:
		return ""
	}
}

func (s *SettingsScreen) Config() config.Config {
	if s == nil {
		return config.Default()
	}
	return s.cfg
}
