package ui

import (
	"fmt"
	"strings"

	"github.com/carroarmato0/nextui-bmo/internal/config"
)

type ProviderMenu struct {
	title       string
	cfg         config.Config
	focus       int
	editing     bool
	editingKind string
	draft       string
}

func NewProviderMenu(cfg config.Config) *ProviderMenu {
	cfg.Normalize()
	if !cfg.UsesAI() {
		cfg.Mode = config.ModeAI
	}
	return &ProviderMenu{title: "AI SETUP", cfg: cfg}
}

func (m *ProviderMenu) Title() string {
	if m == nil || strings.TrimSpace(m.title) == "" {
		return "AI SETUP"
	}
	return strings.ToUpper(strings.TrimSpace(m.title))
}

func (m *ProviderMenu) Move(delta int) {
	if m == nil || m.editing {
		return
	}
	count := m.itemCount()
	if count == 0 {
		m.focus = 0
		return
	}
	m.focus = (m.focus + delta) % count
	if m.focus < 0 {
		m.focus += count
	}
}

func (m *ProviderMenu) ToggleFocused() error {
	if m == nil {
		return fmt.Errorf("nil provider menu")
	}
	if m.editing {
		return m.SubmitEdit()
	}
	switch m.focus {
	case 0:
		if m.cfg.Mode == config.ModeIdle {
			m.cfg.Mode = config.ModeAI
		} else {
			m.cfg.Mode = config.ModeIdle
		}
		if m.cfg.Mode == config.ModeIdle {
			m.cfg.STT = config.Provider{}
			m.cfg.Chat = config.Provider{}
			m.cfg.TTS = config.Provider{}
		}
	case 1:
		m.cfg.STT = cycleProviderPreset(m.cfg.STT, []providerPreset{{Name: "openai-compatible", Model: "whisper-1"}, {Name: "local", Model: "whisper.cpp"}, {Name: "custom", Model: ""}})
	case 2:
		return m.BeginAPIKeyEdit("stt")
	case 3:
		m.cfg.Chat = cycleProviderPreset(m.cfg.Chat, []providerPreset{{Name: "openai-compatible", Model: "gpt-4o-mini"}, {Name: "local", Model: "llama-3.2-3b-instruct"}, {Name: "custom", Model: ""}})
	case 4:
		return m.BeginAPIKeyEdit("chat")
	case 5:
		m.cfg.TTS = cycleProviderPreset(m.cfg.TTS, []providerPreset{{Name: "openai-compatible", Model: "tts-1", Voice: "alloy"}, {Name: "local", Model: "piper"}, {Name: "custom", Model: ""}})
	case 6:
		return m.BeginAPIKeyEdit("tts")
	default:
		return fmt.Errorf("unsupported focus")
	}
	return nil
}

func (m *ProviderMenu) Save() (config.Config, error) {
	if m == nil {
		return config.Config{}, fmt.Errorf("nil provider menu")
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

func (m *ProviderMenu) Overlay() OverlayState {
	if m == nil {
		return OverlayState{}
	}
	items := []OverlayItem{
		{Code: "mode", Label: "MODE: " + strings.ToUpper(m.cfg.Mode), Selected: true, Focused: m.focus == 0 && !m.editing},
		{Code: "stt_provider", Label: providerSummaryLabel("STT", m.cfg.STT), Selected: m.cfg.STT.IsConfigured(), Focused: m.focus == 1 && !m.editing},
		{Code: "stt_key", Label: providerKeyLabel("STT", m.cfg.STT.APIKey, m.editing && m.editingKind == "stt", m.draft), Selected: strings.TrimSpace(m.cfg.STT.APIKey) != "", Focused: m.focus == 2 && !m.editing},
		{Code: "chat_provider", Label: providerSummaryLabel("CHAT", m.cfg.Chat), Selected: m.cfg.Chat.IsConfigured(), Focused: m.focus == 3 && !m.editing},
		{Code: "chat_key", Label: providerKeyLabel("CHAT", m.cfg.Chat.APIKey, m.editing && m.editingKind == "chat", m.draft), Selected: strings.TrimSpace(m.cfg.Chat.APIKey) != "", Focused: m.focus == 4 && !m.editing},
		{Code: "tts_provider", Label: providerSummaryLabel("TTS", m.cfg.TTS), Selected: m.cfg.TTS.IsConfigured(), Focused: m.focus == 5 && !m.editing},
		{Code: "tts_key", Label: providerKeyLabel("TTS", m.cfg.TTS.APIKey, m.editing && m.editingKind == "tts", m.draft), Selected: strings.TrimSpace(m.cfg.TTS.APIKey) != "", Focused: m.focus == 6 && !m.editing},
	}
	subtitle := []string{"SELECT AI PROVIDERS OR API KEYS", "ENTER TO TOGGLE/CYCLE, E TO EDIT KEY"}
	footer := "SPACE/A TO TOGGLE, ENTER TO EDIT KEY, START TO SAVE"
	if m.editing {
		cur := strings.ToUpper(strings.TrimSpace(m.editingKind))
		subtitle = []string{"EDITING " + cur + " API KEY", "ENTER TO SAVE, ESC TO CANCEL, BACKSPACE TO DELETE"}
		footer = "TYPE THE KEY NOW"
	}
	return OverlayState{Visible: true, Title: m.title, Subtitle: subtitle, Items: items, Footer: footer}
}

func (m *ProviderMenu) Config() config.Config {
	if m == nil {
		return config.Default()
	}
	return m.cfg
}

func (m *ProviderMenu) SetAPIKey(kind, key string) error {
	if m == nil {
		return fmt.Errorf("nil provider menu")
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

func (m *ProviderMenu) IsEditing() bool {
	return m != nil && m.editing
}

func (m *ProviderMenu) EditingKind() string {
	if m == nil {
		return ""
	}
	return m.editingKind
}

func (m *ProviderMenu) EditBuffer() string {
	if m == nil {
		return ""
	}
	return m.draft
}

func (m *ProviderMenu) BeginAPIKeyEdit(kind string) error {
	if m == nil {
		return fmt.Errorf("nil provider menu")
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

func (m *ProviderMenu) InsertText(text string) {
	if m == nil || !m.editing {
		return
	}
	m.draft += text
}

func (m *ProviderMenu) Backspace() {
	if m == nil || !m.editing || m.draft == "" {
		return
	}
	r := []rune(m.draft)
	m.draft = string(r[:len(r)-1])
}

func (m *ProviderMenu) SubmitEdit() error {
	if m == nil {
		return fmt.Errorf("nil provider menu")
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

func (m *ProviderMenu) CancelEdit() {
	if m == nil {
		return
	}
	m.editing = false
	m.editingKind = ""
	m.draft = ""
}

func (m *ProviderMenu) itemCount() int { return 7 }

func (m *ProviderMenu) currentAPIKey(kind string) string {
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

type providerPreset struct {
	Name  string
	Model string
	Voice string
}

func cycleProviderPreset(current config.Provider, presets []providerPreset) config.Provider {
	if len(presets) == 0 {
		return current
	}
	currentName := strings.ToLower(strings.TrimSpace(current.Name))
	currentModel := strings.ToLower(strings.TrimSpace(current.Model))
	currentVoice := strings.ToLower(strings.TrimSpace(current.Voice))
	for i, preset := range presets {
		if strings.ToLower(preset.Name) == currentName && strings.ToLower(preset.Model) == currentModel && strings.ToLower(preset.Voice) == currentVoice {
			next := presets[(i+1)%len(presets)]
			return applyProviderPreset(current, next)
		}
	}
	return applyProviderPreset(current, presets[0])
}

func applyProviderPreset(base config.Provider, preset providerPreset) config.Provider {
	base.Name = preset.Name
	base.Model = preset.Model
	base.Voice = preset.Voice
	return base
}

func providerSummaryLabel(kind string, p config.Provider) string {
	name := strings.TrimSpace(p.Name)
	model := strings.TrimSpace(p.Model)
	voice := strings.TrimSpace(p.Voice)
	if name == "" || model == "" {
		return kind + ": NOT SET"
	}
	parts := []string{kind + ": " + strings.ToUpper(name), model}
	if voice != "" {
		parts = append(parts, "VOICE "+voice)
	}
	if strings.TrimSpace(p.APIKey) == "" {
		parts = append(parts, "KEY MISSING")
	} else {
		parts = append(parts, "KEY SET")
	}
	return strings.Join(parts, " • ")
}

func providerKeyLabel(kind, key string, editing bool, draft string) string {
	if editing {
		return kind + ": KEY EDITING"
	}
	if strings.TrimSpace(key) == "" {
		return kind + ": KEY MISSING"
	}
	return kind + ": KEY SET"
}
