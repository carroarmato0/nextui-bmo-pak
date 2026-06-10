package ui

import (
	"fmt"
	"strings"

	"github.com/carroarmato0/nextui-bmo/internal/config"
)

type ProviderMenu struct {
	title string
	cfg   config.Config
	focus int
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
	if m == nil {
		return
	}
	count := 4
	m.focus = (m.focus + delta) % count
	if m.focus < 0 {
		m.focus += count
	}
}

func (m *ProviderMenu) ToggleFocused() error {
	if m == nil {
		return fmt.Errorf("nil provider menu")
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
		m.cfg.Chat = cycleProviderPreset(m.cfg.Chat, []providerPreset{{Name: "openai-compatible", Model: "gpt-4o-mini"}, {Name: "local", Model: "llama-3.2-3b-instruct"}, {Name: "custom", Model: ""}})
	case 3:
		m.cfg.TTS = cycleProviderPreset(m.cfg.TTS, []providerPreset{{Name: "openai-compatible", Model: "tts-1", Voice: "alloy"}, {Name: "local", Model: "piper"}, {Name: "custom", Model: ""}})
	default:
		return fmt.Errorf("unsupported focus")
	}
	return nil
}

func (m *ProviderMenu) Save() (config.Config, error) {
	if m == nil {
		return config.Config{}, fmt.Errorf("nil provider menu")
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
		{Code: "mode", Label: "MODE: " + strings.ToUpper(m.cfg.Mode), Selected: true, Focused: m.focus == 0},
		{Code: "stt", Label: providerSummaryLabel("STT", m.cfg.STT), Selected: m.cfg.STT.IsConfigured(), Focused: m.focus == 1},
		{Code: "chat", Label: providerSummaryLabel("CHAT", m.cfg.Chat), Selected: m.cfg.Chat.IsConfigured(), Focused: m.focus == 2},
		{Code: "tts", Label: providerSummaryLabel("TTS", m.cfg.TTS), Selected: m.cfg.TTS.IsConfigured(), Focused: m.focus == 3},
	}
	return OverlayState{
		Visible:  true,
		Title:    m.title,
		Subtitle: []string{"SELECT AI PROVIDERS", "ENTER TO CYCLE PROFILE"},
		Items:    items,
		Footer:   "SPACE TO TOGGLE MODE/PROFILE • START TO SAVE",
	}
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
