package ui

import (
	"fmt"
	"strings"

	"github.com/carroarmato0/nextui-bmo/internal/config"
)

var logLevelOrder = []string{"debug", "info", "warn", "error"}

type SettingsMenu struct {
	title            string
	cfg              config.Config
	focus            int
	onLogLevelChange func(string)
	onRestore        func() error
}

func NewSettingsMenu(cfg config.Config) *SettingsMenu {
	cfg.Normalize()
	return &SettingsMenu{title: "SETTINGS", cfg: cfg}
}

// SetLogLevelCallback registers a function called immediately whenever the
// log level item is cycled, so the live logger can be updated in place.
func (m *SettingsMenu) SetLogLevelCallback(fn func(string)) {
	if m != nil {
		m.onLogLevelChange = fn
	}
}

// SetRestoreDefaultsCallback registers the action run when RESTORE DEFAULTS is activated.
func (m *SettingsMenu) SetRestoreDefaultsCallback(fn func() error) {
	if m != nil {
		m.onRestore = fn
	}
}

func (m *SettingsMenu) Title() string {
	if m == nil || strings.TrimSpace(m.title) == "" {
		return "SETTINGS"
	}
	return strings.ToUpper(strings.TrimSpace(m.title))
}

// Move advances the focus by delta, skipping non-navigable slots.
// Slots 3–5 (AI status indicators) are always skipped.
// Slot 1 (log system prompt) is skipped unless log level is "debug".
func (m *SettingsMenu) Move(delta int) {
	if m == nil {
		return
	}
	const count = 14
	step := 1
	if delta < 0 {
		step = -1
	}
	m.focus = ((m.focus + delta) % count + count) % count
	for m.shouldSkip(m.focus) {
		m.focus = (m.focus + step + count) % count
	}
}

func (m *SettingsMenu) shouldSkip(idx int) bool {
	if idx >= 3 && idx <= 5 {
		return true
	}
	if idx == 1 && strings.ToLower(strings.TrimSpace(m.cfg.LogLevel)) != "debug" {
		return true
	}
	return false
}

func (m *SettingsMenu) ToggleFocused() error {
	if m == nil {
		return fmt.Errorf("nil settings menu")
	}
	switch m.focus {
	case 0:
		curr := strings.ToLower(strings.TrimSpace(m.cfg.LogLevel))
		next := logLevelOrder[0]
		for i, l := range logLevelOrder {
			if l == curr {
				next = logLevelOrder[(i+1)%len(logLevelOrder)]
				break
			}
		}
		m.cfg.LogLevel = next
		if m.onLogLevelChange != nil {
			m.onLogLevelChange(next)
		}
	case 1:
		m.cfg.LogSystemPrompt = !m.cfg.LogSystemPrompt
	case 2:
		if m.cfg.Mode == config.ModeIdle {
			m.cfg.Mode = config.ModeAI
		} else {
			m.cfg.Mode = config.ModeIdle
		}
	case 6:
		m.cfg.DeviceContext.Library = !m.cfg.DeviceContext.Library
	case 7:
		m.cfg.DeviceContext.Saves = !m.cfg.DeviceContext.Saves
	case 8:
		m.cfg.DeviceContext.PlayLog = !m.cfg.DeviceContext.PlayLog
	case 9:
		m.cfg.DeviceContext.System = !m.cfg.DeviceContext.System
	case 10:
		m.cfg.DeviceContext.Achievements = !m.cfg.DeviceContext.Achievements
	case 11:
		if m.cfg.LibraryDetail == config.LibraryDetailRandom {
			m.cfg.LibraryDetail = config.LibraryDetailFull
		} else {
			m.cfg.LibraryDetail = config.LibraryDetailRandom
		}
	case 12:
		levels := config.SupportedProactiveTalkLevels()
		curr := strings.ToLower(strings.TrimSpace(m.cfg.ProactiveTalk))
		next := levels[0]
		for i, l := range levels {
			if l == curr {
				next = levels[(i+1)%len(levels)]
				break
			}
		}
		m.cfg.ProactiveTalk = next
	case 13:
		if m.onRestore != nil {
			return m.onRestore()
		}
	default:
		return fmt.Errorf("unsupported focus %d", m.focus)
	}
	return nil
}

func (m *SettingsMenu) Overlay() OverlayState {
	isDebug := strings.ToLower(strings.TrimSpace(m.cfg.LogLevel)) == "debug"
	isAI := m.cfg.Mode == config.ModeAI
	items := []OverlayItem{
		{Code: "log_level", Label: "LOG: " + strings.ToUpper(m.cfg.LogLevel),
			Selected: true, Focused: m.focus == 0},
		{Code: "log_system_prompt", Label: "LOG SYSTEM PROMPT: " + onOff(m.cfg.LogSystemPrompt),
			Selected: m.cfg.LogSystemPrompt, Focused: m.focus == 1, Hidden: !isDebug},
		{Code: "mode", Label: "MODE: " + strings.ToUpper(m.cfg.Mode),
			Selected: true, Focused: m.focus == 2},
		{Code: "stt_status", Label: providerSummaryLabel("STT", m.cfg.STT), Disabled: !isAI},
		{Code: "chat_status", Label: providerSummaryLabel("CHAT", m.cfg.Chat), Disabled: !isAI},
		{Code: "tts_status", Label: providerSummaryLabel("TTS", m.cfg.TTS), Disabled: !isAI},
		{Code: "aware_library", Label: "AWARE LIBRARY: " + onOff(m.cfg.DeviceContext.Library),
			Selected: m.cfg.DeviceContext.Library, Focused: m.focus == 6},
		{Code: "aware_saves", Label: "AWARE SAVES: " + onOff(m.cfg.DeviceContext.Saves),
			Selected: m.cfg.DeviceContext.Saves, Focused: m.focus == 7},
		{Code: "aware_playlog", Label: "AWARE PLAY LOG: " + onOff(m.cfg.DeviceContext.PlayLog),
			Selected: m.cfg.DeviceContext.PlayLog, Focused: m.focus == 8},
		{Code: "aware_system", Label: "AWARE SYSTEM: " + onOff(m.cfg.DeviceContext.System),
			Selected: m.cfg.DeviceContext.System, Focused: m.focus == 9},
		{Code: "aware_achievements", Label: "AWARE ACHIEVEMENTS: " + onOff(m.cfg.DeviceContext.Achievements),
			Selected: m.cfg.DeviceContext.Achievements, Focused: m.focus == 10},
		{Code: "library_detail", Label: "LIBRARY DETAIL: " + strings.ToUpper(m.cfg.LibraryDetail),
			Selected: true, Focused: m.focus == 11},
		{Code: "proactive_talk", Label: "PROACTIVE TALK: " + strings.ToUpper(m.cfg.ProactiveTalk),
			Selected: true, Focused: m.focus == 12},
		{Code: "restore_defaults", Label: "RESTORE DEFAULTS", Focused: m.focus == 13},
	}
	return OverlayState{Visible: true, Title: m.title, Items: items}
}

func (m *SettingsMenu) Save() (config.Config, error) {
	if m == nil {
		return config.Config{}, fmt.Errorf("nil settings menu")
	}
	cfg := m.cfg
	cfg.Normalize()
	cfg.SetupComplete = true
	if err := cfg.Validate(); err != nil {
		return config.Config{}, err
	}
	m.cfg = cfg
	return cfg, nil
}

func (m *SettingsMenu) Config() config.Config {
	if m == nil {
		return config.Default()
	}
	return m.cfg
}

func (m *SettingsMenu) SetProvider(kind string, provider config.Provider) {
	if m == nil {
		return
	}
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case providerKindSTT:
		m.cfg.STT = provider
	case providerKindChat:
		m.cfg.Chat = provider
	case providerKindTTS:
		m.cfg.TTS = provider
	}
}

func onOff(v bool) string {
	if v {
		return "ON"
	}
	return "OFF"
}
