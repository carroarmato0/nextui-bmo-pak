package ui

import (
	"fmt"
	"strings"

	"github.com/carroarmato0/nextui-bmo/internal/config"
)

var logLevelOrder = []string{"debug", "info", "warn", "error"}

// ModChoice is one selectable mod in the MOD cycle item. ID is persisted to
// config.ActiveMod; Label is the already-formatted display string.
type ModChoice struct {
	ID    string
	Label string
}

type SettingsMenu struct {
	title            string
	cfg              config.Config
	focus            int
	onLogLevelChange func(string)
	onRestore        func() error
	modChoices       []ModChoice
	onModChange      func(string)
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

// SetModChoices supplies the selectable mods shown by the MOD item.
func (m *SettingsMenu) SetModChoices(choices []ModChoice) {
	if m != nil {
		m.modChoices = choices
	}
}

// SetModChangeCallback registers a function called when the active mod is
// cycled, so the app can reload persona/voice/quotes/faces/audio in place.
func (m *SettingsMenu) SetModChangeCallback(fn func(string)) {
	if m != nil {
		m.onModChange = fn
	}
}

// cycleMod advances the active mod to the next choice, wrapping around, and
// fires the change callback.
func (m *SettingsMenu) cycleMod() {
	if len(m.modChoices) == 0 {
		return
	}
	idx := 0
	for i, c := range m.modChoices {
		if c.ID == m.cfg.ActiveMod {
			idx = i
			break
		}
	}
	next := m.modChoices[(idx+1)%len(m.modChoices)]
	m.cfg.ActiveMod = next.ID
	if m.onModChange != nil {
		m.onModChange(next.ID)
	}
}

// modLabel returns the display label for the currently active mod.
func (m *SettingsMenu) modLabel() string {
	for _, c := range m.modChoices {
		if c.ID == m.cfg.ActiveMod {
			return c.Label
		}
	}
	if len(m.modChoices) > 0 {
		return m.modChoices[0].Label // active id not found: show the default
	}
	return "BMO (DEFAULT)"
}

func (m *SettingsMenu) Title() string {
	if m == nil || strings.TrimSpace(m.title) == "" {
		return "SETTINGS"
	}
	return strings.ToUpper(strings.TrimSpace(m.title))
}

// Move advances the focus by delta, skipping non-navigable slots.
// Slots 3–6 (AI status indicators) are always skipped.
// Slot 1 (log system prompt) is skipped unless log level is "debug".
func (m *SettingsMenu) Move(delta int) {
	if m == nil {
		return
	}
	const count = 17
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
	isAI := m.cfg.Mode == config.ModeAI
	if idx >= 3 && idx <= 5 {
		return !isAI // provider rows focusable only in AI mode
	}
	if idx == 6 {
		return true // voice is a read-only status row
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
	case 7:
		m.cfg.DeviceContext.Library = !m.cfg.DeviceContext.Library
	case 8:
		m.cfg.DeviceContext.Saves = !m.cfg.DeviceContext.Saves
	case 9:
		m.cfg.DeviceContext.PlayLog = !m.cfg.DeviceContext.PlayLog
	case 10:
		m.cfg.DeviceContext.System = !m.cfg.DeviceContext.System
	case 11:
		m.cfg.DeviceContext.Achievements = !m.cfg.DeviceContext.Achievements
	case 12:
		if m.cfg.LibraryDetail == config.LibraryDetailRandom {
			m.cfg.LibraryDetail = config.LibraryDetailFull
		} else {
			m.cfg.LibraryDetail = config.LibraryDetailRandom
		}
	case 13:
		timeouts := config.SupportedRequestTimeouts()
		curr := m.cfg.RequestTimeout
		next := timeouts[0]
		for i, v := range timeouts {
			if v == curr {
				next = timeouts[(i+1)%len(timeouts)]
				break
			}
		}
		m.cfg.RequestTimeout = next
	case 14:
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
	case 15:
		m.cycleMod()
	case 16:
		if m.onRestore != nil {
			return m.onRestore()
		}
	default:
		return fmt.Errorf("unsupported focus %d", m.focus)
	}
	return nil
}

// Cycle changes the focused setting. For provider rows (stt/chat/tts) it moves
// the active provider forward (delta>0) or backward (delta<0). For every other
// row it ignores the sign and advances forward, matching ToggleFocused, so the
// renderer's LEFT and RIGHT both cycle non-provider items as before.
func (m *SettingsMenu) Cycle(delta int) error {
	if m == nil {
		return fmt.Errorf("nil settings menu")
	}
	switch m.focus {
	case 3:
		m.cfg.STT.Cycle(delta)
		return nil
	case 4:
		m.cfg.Chat.Cycle(delta)
		return nil
	case 5:
		m.cfg.TTS.Cycle(delta)
		return nil
	default:
		return m.ToggleFocused()
	}
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
		{Code: "stt_status", Label: providerModelLabel("STT", m.cfg.STT.Current()), Disabled: !isAI, Focused: m.focus == 3},
		{Code: "chat_status", Label: providerModelLabel("CHAT", m.cfg.Chat.Current()), Disabled: !isAI, Focused: m.focus == 4},
		{Code: "tts_status", Label: providerModelLabel("TTS", m.cfg.TTS.Current()), Disabled: !isAI, Focused: m.focus == 5},
		{Code: "voice_status", Label: voiceStatusLabel(m.cfg.TTS.Current()), Disabled: !isAI},
		{Code: "aware_library", Label: "AWARE LIBRARY: " + onOff(m.cfg.DeviceContext.Library),
			Selected: m.cfg.DeviceContext.Library, Focused: m.focus == 7},
		{Code: "aware_saves", Label: "AWARE SAVES: " + onOff(m.cfg.DeviceContext.Saves),
			Selected: m.cfg.DeviceContext.Saves, Focused: m.focus == 8},
		{Code: "aware_playlog", Label: "AWARE PLAY LOG: " + onOff(m.cfg.DeviceContext.PlayLog),
			Selected: m.cfg.DeviceContext.PlayLog, Focused: m.focus == 9},
		{Code: "aware_system", Label: "AWARE SYSTEM: " + onOff(m.cfg.DeviceContext.System),
			Selected: m.cfg.DeviceContext.System, Focused: m.focus == 10},
		{Code: "aware_achievements", Label: "AWARE ACHIEVEMENTS: " + onOff(m.cfg.DeviceContext.Achievements),
			Selected: m.cfg.DeviceContext.Achievements, Focused: m.focus == 11},
		{Code: "library_detail", Label: "LIBRARY DETAIL: " + strings.ToUpper(m.cfg.LibraryDetail),
			Selected: true, Focused: m.focus == 12},
		{Code: "request_timeout", Label: fmt.Sprintf("TIMEOUT: %ds", m.cfg.RequestTimeout),
			Selected: true, Focused: m.focus == 13},
		{Code: "proactive_talk", Label: "PROACTIVE TALK: " + strings.ToUpper(m.cfg.ProactiveTalk),
			Selected: true, Focused: m.focus == 14},
		{Code: "mod", Label: "MOD: " + m.modLabel(),
			Selected: true, Focused: m.focus == 15},
		{Code: "restore_defaults", Label: "RESTORE DEFAULTS", Focused: m.focus == 16},
	}
	// FocusIndex is the index into the VISIBLE (non-Hidden) row list so the
	// renderer's scroll viewport stays correct when the debug-only
	// log_system_prompt row is hidden.
	focusVisible := 0
	visible := 0
	for i := range items {
		if items[i].Hidden {
			continue
		}
		if i == m.focus {
			focusVisible = visible
		}
		visible++
	}
	return OverlayState{
		Visible:    true,
		Title:      m.title,
		Subtitle:   []string{"UP/DOWN: NAVIGATE", "LEFT/RIGHT: CYCLE (AUTO-SAVED)"},
		Footer:     "START OR B TO CLOSE",
		Items:      items,
		FocusIndex: focusVisible,
	}
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
	set := config.ProviderSet{Active: provider.Name, Providers: []config.Provider{provider}}
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case providerKindSTT:
		m.cfg.STT = set
	case providerKindChat:
		m.cfg.Chat = set
	case providerKindTTS:
		m.cfg.TTS = set
	}
}

func onOff(v bool) string {
	if v {
		return "ON"
	}
	return "OFF"
}

func providerModelLabel(kind string, p config.Provider) string {
	model := strings.TrimSpace(p.Model)
	if model == "" {
		return kind + ": NOT SET"
	}
	return kind + ": " + model
}

func voiceStatusLabel(p config.Provider) string {
	voice := strings.TrimSpace(p.Voice)
	if voice == "" {
		return "VOICE: NOT SET"
	}
	return "VOICE: " + voice
}
