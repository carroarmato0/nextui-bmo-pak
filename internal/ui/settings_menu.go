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
	editing          bool
	editingKind      string
	draft            string
	onLogLevelChange func(string)
	onRestore        func() error
}

func NewSettingsMenu(cfg config.Config) *SettingsMenu {
	cfg.Normalize()
	return &SettingsMenu{title: "SETTINGS", cfg: cfg}
}

// SetLogLevelCallback registers a function called immediately whenever the
// log level item is cycled, so the running logger can be updated without
// waiting for Save.
func (m *SettingsMenu) SetLogLevelCallback(fn func(string)) {
	if m != nil {
		m.onLogLevelChange = fn
	}
}

// SetRestoreDefaultsCallback registers the action run when the RESTORE
// DEFAULTS item is activated (rewrites the persona/voice prompt files with
// their built-in defaults).
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

func (m *SettingsMenu) Move(delta int) {
	if m == nil || m.editing {
		return
	}
	const count = 12
	m.focus = (m.focus + delta) % count
	if m.focus < 0 {
		m.focus += count
	}
}

func (m *SettingsMenu) ToggleFocused() error {
	if m == nil {
		return fmt.Errorf("nil settings menu")
	}
	if m.editing {
		return m.SubmitEdit()
	}
	switch m.focus {
	case 0: // log level — cycle through ordered list
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
	case 1: // mode — toggle between idle and ai
		if m.cfg.Mode == config.ModeIdle {
			m.cfg.Mode = config.ModeAI
		} else {
			m.cfg.Mode = config.ModeIdle
		}
	case 2:
		return m.BeginAPIKeyEdit(providerKindSTT)
	case 3:
		return m.BeginAPIKeyEdit(providerKindChat)
	case 4:
		return m.BeginAPIKeyEdit(providerKindTTS)
	case 5:
		m.cfg.DeviceContext.Library = !m.cfg.DeviceContext.Library
	case 6:
		m.cfg.DeviceContext.Saves = !m.cfg.DeviceContext.Saves
	case 7:
		m.cfg.DeviceContext.PlayLog = !m.cfg.DeviceContext.PlayLog
	case 8:
		m.cfg.DeviceContext.System = !m.cfg.DeviceContext.System
	case 9:
		m.cfg.DeviceContext.Achievements = !m.cfg.DeviceContext.Achievements
	case 10: // proactive talk — cycle through supported levels
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
	case 11: // restore persona/voice prompt files to built-in defaults
		if m.onRestore != nil {
			return m.onRestore()
		}
	default:
		return fmt.Errorf("unsupported focus %d", m.focus)
	}
	return nil
}

func (m *SettingsMenu) Save() (config.Config, error) {
	if m == nil {
		return config.Config{}, fmt.Errorf("nil settings menu")
	}
	if m.editing {
		if err := m.SubmitEdit(); err != nil {
			return config.Config{}, err
		}
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

func (m *SettingsMenu) Overlay() OverlayState {
	if m == nil {
		return OverlayState{}
	}
	items := []OverlayItem{
		{Code: "log_level", Label: "LOG: " + strings.ToUpper(m.cfg.LogLevel),
			Selected: true, Focused: m.focus == 0 && !m.editing},
		{Code: "mode", Label: "MODE: " + strings.ToUpper(m.cfg.Mode),
			Selected: true, Focused: m.focus == 1 && !m.editing},
		{Code: "stt_key",
			Label:    providerKeyLabel("STT", m.cfg.STT.APIKey, m.editing && m.editingKind == providerKindSTT),
			Selected: strings.TrimSpace(m.cfg.STT.APIKey) != "",
			Focused:  m.focus == 2 && !m.editing},
		{Code: "chat_key",
			Label:    providerKeyLabel("CHAT", m.cfg.Chat.APIKey, m.editing && m.editingKind == providerKindChat),
			Selected: strings.TrimSpace(m.cfg.Chat.APIKey) != "",
			Focused:  m.focus == 3 && !m.editing},
		{Code: "tts_key",
			Label:    providerKeyLabel("TTS", m.cfg.TTS.APIKey, m.editing && m.editingKind == providerKindTTS),
			Selected: strings.TrimSpace(m.cfg.TTS.APIKey) != "",
			Focused:  m.focus == 4 && !m.editing},
		{Code: "aware_library", Label: "AWARE LIBRARY: " + onOff(m.cfg.DeviceContext.Library),
			Selected: m.cfg.DeviceContext.Library, Focused: m.focus == 5 && !m.editing},
		{Code: "aware_saves", Label: "AWARE SAVES: " + onOff(m.cfg.DeviceContext.Saves),
			Selected: m.cfg.DeviceContext.Saves, Focused: m.focus == 6 && !m.editing},
		{Code: "aware_playlog", Label: "AWARE PLAY LOG: " + onOff(m.cfg.DeviceContext.PlayLog),
			Selected: m.cfg.DeviceContext.PlayLog, Focused: m.focus == 7 && !m.editing},
		{Code: "aware_system", Label: "AWARE SYSTEM: " + onOff(m.cfg.DeviceContext.System),
			Selected: m.cfg.DeviceContext.System, Focused: m.focus == 8 && !m.editing},
		{Code: "aware_achievements", Label: "AWARE ACHIEVEMENTS: " + onOff(m.cfg.DeviceContext.Achievements),
			Selected: m.cfg.DeviceContext.Achievements, Focused: m.focus == 9 && !m.editing},
		{Code: "proactive_talk", Label: "PROACTIVE TALK: " + strings.ToUpper(m.cfg.ProactiveTalk),
			Selected: m.cfg.ProactiveTalk != config.ProactiveOff, Focused: m.focus == 10 && !m.editing},
		{Code: "restore_defaults", Label: "RESTORE DEFAULTS",
			Selected: true, Focused: m.focus == 11 && !m.editing},
	}
	subtitle := []string{"UP/DOWN: NAVIGATE", "LEFT/RIGHT: CYCLE (AUTO-SAVED)"}
	footer := "START OR B TO CLOSE"
	if m.editing {
		cur := strings.ToUpper(strings.TrimSpace(m.editingKind))
		subtitle = []string{"EDITING " + cur + " API KEY", "START TO SAVE  B TO CANCEL"}
		footer = "TYPE THE KEY NOW"
	}
	return OverlayState{Visible: true, Title: m.title, Subtitle: subtitle, Items: items, Footer: footer}
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

func (m *SettingsMenu) SetAPIKey(kind, key string) error {
	if m == nil {
		return fmt.Errorf("nil settings menu")
	}
	key = strings.TrimSpace(key)
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case providerKindSTT:
		m.cfg.STT.APIKey = key
	case providerKindChat:
		m.cfg.Chat.APIKey = key
	case providerKindTTS:
		m.cfg.TTS.APIKey = key
	default:
		return fmt.Errorf("unknown provider kind %q", kind)
	}
	return nil
}

func (m *SettingsMenu) IsEditing() bool { return m != nil && m.editing }
func (m *SettingsMenu) EditingKind() string {
	if m == nil {
		return ""
	}
	return m.editingKind
}
func (m *SettingsMenu) EditBuffer() string {
	if m == nil {
		return ""
	}
	return m.draft
}
func (m *SettingsMenu) BeginAPIKeyEdit(kind string) error {
	if m == nil {
		return fmt.Errorf("nil settings menu")
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	switch kind {
	case providerKindSTT, providerKindChat, providerKindTTS:
	default:
		return fmt.Errorf("unknown provider kind %q", kind)
	}
	m.editing = true
	m.editingKind = kind
	m.draft = m.currentAPIKey(kind)
	return nil
}
func (m *SettingsMenu) InsertText(text string) {
	if m == nil || !m.editing {
		return
	}
	m.draft += text
}
func (m *SettingsMenu) Backspace() {
	if m == nil || !m.editing || m.draft == "" {
		return
	}
	r := []rune(m.draft)
	m.draft = string(r[:len(r)-1])
}
func (m *SettingsMenu) SubmitEdit() error {
	if m == nil {
		return fmt.Errorf("nil settings menu")
	}
	if !m.editing {
		return nil
	}
	if err := m.SetAPIKey(m.editingKind, m.draft); err != nil {
		return err
	}
	m.editing = false
	m.editingKind = ""
	m.draft = ""
	return nil
}
func (m *SettingsMenu) CancelEdit() {
	if m == nil {
		return
	}
	m.editing = false
	m.editingKind = ""
	m.draft = ""
}
func (m *SettingsMenu) currentAPIKey(kind string) string {
	switch kind {
	case providerKindSTT:
		return m.cfg.STT.APIKey
	case providerKindChat:
		return m.cfg.Chat.APIKey
	case providerKindTTS:
		return m.cfg.TTS.APIKey
	default:
		return ""
	}
}

func onOff(v bool) string {
	if v {
		return "ON"
	}
	return "OFF"
}
