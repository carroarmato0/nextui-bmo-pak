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
	const count = 5
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
		return m.BeginAPIKeyEdit("stt")
	case 3:
		return m.BeginAPIKeyEdit("chat")
	case 4:
		return m.BeginAPIKeyEdit("tts")
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
			Label:    providerKeyLabel("STT", m.cfg.STT.APIKey, m.editing && m.editingKind == "stt", m.draft),
			Selected: strings.TrimSpace(m.cfg.STT.APIKey) != "",
			Focused:  m.focus == 2 && !m.editing},
		{Code: "chat_key",
			Label:    providerKeyLabel("CHAT", m.cfg.Chat.APIKey, m.editing && m.editingKind == "chat", m.draft),
			Selected: strings.TrimSpace(m.cfg.Chat.APIKey) != "",
			Focused:  m.focus == 3 && !m.editing},
		{Code: "tts_key",
			Label:    providerKeyLabel("TTS", m.cfg.TTS.APIKey, m.editing && m.editingKind == "tts", m.draft),
			Selected: strings.TrimSpace(m.cfg.TTS.APIKey) != "",
			Focused:  m.focus == 4 && !m.editing},
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
	case "stt":
		m.cfg.STT = provider
	case "chat":
		m.cfg.Chat = provider
	case "tts":
		m.cfg.TTS = provider
	}
}

func (m *SettingsMenu) SetAPIKey(kind, key string) error {
	if m == nil {
		return fmt.Errorf("nil settings menu")
	}
	key = strings.TrimSpace(key)
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "stt":
		m.cfg.STT.APIKey = key
	case "chat":
		m.cfg.Chat.APIKey = key
	case "tts":
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
	case "stt", "chat", "tts":
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
	case "stt":
		return m.cfg.STT.APIKey
	case "chat":
		return m.cfg.Chat.APIKey
	case "tts":
		return m.cfg.TTS.APIKey
	default:
		return ""
	}
}
