package ui

import (
	"fmt"
	"strings"

	"github.com/carroarmato0/nextui-bmo/internal/config"
)

type ScreenID string

const (
	ScreenMain    ScreenID = "main"
	ScreenSetup   ScreenID = "setup"
	ScreenSettings ScreenID = "settings"
)

type SetupFlow struct {
	cfg config.Config
}

func NewSetupFlow(cfg config.Config) *SetupFlow {
	cfg.Normalize()
	return &SetupFlow{cfg: cfg}
}

func (f *SetupFlow) InitialScreen() ScreenID {
	if f == nil {
		return ScreenMain
	}
	if !f.cfg.SetupComplete {
		return ScreenSetup
	}
	if f.cfg.UsesAI() && (!f.cfg.STT.IsConfigured() || !f.cfg.Chat.IsConfigured() || !f.cfg.TTS.IsConfigured()) {
		return ScreenSetup
	}
	return ScreenMain
}

func (f *SetupFlow) SetupScreen() *SetupScreen {
	if f == nil {
		return NewSetupScreen(config.Default())
	}
	return NewSetupScreen(f.cfg)
}

type SetupScreen struct {
	cfg config.Config
}

func NewSetupScreen(cfg config.Config) *SetupScreen {
	cfg.Normalize()
	return &SetupScreen{cfg: cfg}
}

func (s *SetupScreen) SelectIdleOnly() {
	s.cfg.Mode = config.ModeIdle
	s.cfg.SetupComplete = false
	s.cfg.STT = config.Provider{}
	s.cfg.Chat = config.Provider{}
	s.cfg.TTS = config.Provider{}
}

func (s *SetupScreen) SelectAIMode() {
	s.cfg.Mode = config.ModeAI
	s.cfg.SetupComplete = false
}

func (s *SetupScreen) SetProvider(kind string, provider config.Provider) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "stt":
		s.cfg.STT = provider
	case "chat":
		s.cfg.Chat = provider
	case "tts":
		s.cfg.TTS = provider
	}
}

func (s *SetupScreen) PTTButtons() []string {
	if s == nil {
		return config.DefaultPTTButtons()
	}
	return append([]string(nil), s.cfg.PTTButtons...)
}

func (s *SetupScreen) SetPTTButtons(buttons []string) error {
	if s == nil {
		return fmt.Errorf("nil setup screen")
	}
	if err := config.ValidatePTTButtons(buttons); err != nil {
		return err
	}
	s.cfg.PTTButtons = config.NormalizePTTButtons(buttons)
	return nil
}

func (s *SetupScreen) ResetPTTButtons() {
	if s == nil {
		return
	}
	s.cfg.PTTButtons = config.DefaultPTTButtons()
}

func (s *SetupScreen) Save() error {
	if s == nil {
		return fmt.Errorf("nil setup screen")
	}
	if err := s.cfg.Validate(); err != nil {
		return err
	}
	if s.cfg.Mode == config.ModeIdle {
		s.cfg.STT = config.Provider{}
		s.cfg.Chat = config.Provider{}
		s.cfg.TTS = config.Provider{}
	}
	s.cfg.SetupComplete = true
	return nil
}

func (s *SetupScreen) Config() config.Config {
	if s == nil {
		return config.Default()
	}
	return s.cfg
}
