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
	about            *AboutState
	aboutActive      bool
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

// SetAbout supplies the (static) About-screen content shown when the ABOUT
// item is activated.
func (m *SettingsMenu) SetAbout(about AboutState) {
	if m != nil {
		a := about
		m.about = &a
	}
}

// AboutActive reports whether the About screen is currently shown in place of
// the settings list.
func (m *SettingsMenu) AboutActive() bool {
	return m != nil && m.aboutActive && m.about != nil
}

// DismissAbout returns from the About screen to the settings list.
func (m *SettingsMenu) DismissAbout() {
	if m != nil {
		m.aboutActive = false
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

// secondsUnit renders as a lowercase "s" via a private-use glyph in the
// renderer's font (drawText force-uppercases ASCII, so a plain "s" would show
// as "S"). Used for the request-timeout unit, e.g. "TIMEOUT: 30s". Keep the
// rune in sync with the glyph table in internal/renderer.
const secondsUnit = "\uE073"

// settingsSlot is one settings row together with whether the cursor may land on
// it. The slot order is fixed (indices are stable) so the ToggleFocused/Cycle
// switches stay aligned; visibility and navigability vary with mode/log level.
type settingsSlot struct {
	item      OverlayItem
	navigable bool
}

const settingsSlotCount = 21

// slots is the single source of truth for the settings layout: row content,
// visibility and navigability all derive from here, so Move, shouldSkip and
// Overlay can never drift out of sync. AI-only rows are hidden (not just
// disabled) when the assistant is not in AI mode, grouping them under MODE.
func (m *SettingsMenu) slots() []settingsSlot {
	isDebug := strings.ToLower(strings.TrimSpace(m.cfg.LogLevel)) == "debug"
	isAI := m.cfg.Mode == config.ModeAI
	aiToggle := func(code, label string, on bool) settingsSlot {
		return settingsSlot{OverlayItem{Code: code, Label: label, Selected: on, Hidden: !isAI, Indent: true}, isAI}
	}
	aiCycle := func(code, label string) settingsSlot {
		return settingsSlot{OverlayItem{Code: code, Label: label, Selected: true, Hidden: !isAI, Indent: true}, isAI}
	}
	// AI provider/voice rows carry a status box like every other entry and are
	// nested under MODE via Indent. navigable is false for the read-only voice.
	aiStatus := func(code, label string, navigable bool) settingsSlot {
		return settingsSlot{OverlayItem{Code: code, Label: label, Selected: true, Hidden: !isAI, Indent: true}, navigable && isAI}
	}
	return []settingsSlot{
		{OverlayItem{Code: "log_level", Label: "LOG: " + strings.ToUpper(m.cfg.LogLevel), Selected: true}, true},
		{OverlayItem{Code: "log_system_prompt", Label: "LOG SYSTEM PROMPT: " + onOff(m.cfg.LogSystemPrompt), Selected: m.cfg.LogSystemPrompt, Hidden: !isDebug}, isDebug},
		{OverlayItem{Code: "mode", Label: "MODE: " + strings.ToUpper(m.cfg.Mode), Selected: true}, true},
		// Provider rows: hidden in idle, focusable (cycled L/R) in AI mode.
		aiStatus("stt_status", providerModelLabel("STT", m.cfg.STT.Current()), true),
		aiStatus("chat_status", providerModelLabel("CHAT", m.cfg.Chat.Current()), true),
		aiStatus("tts_status", providerModelLabel("TTS", m.cfg.TTS.Current()), true),
		// Voice: AI-only, read-only status row (never focusable).
		aiStatus("voice_status", voiceStatusLabel(m.cfg.TTS.Current()), false),
		aiToggle("aware_library", "AWARE LIBRARY: "+onOff(m.cfg.DeviceContext.Library), m.cfg.DeviceContext.Library),
		aiToggle("aware_saves", "AWARE SAVES: "+onOff(m.cfg.DeviceContext.Saves), m.cfg.DeviceContext.Saves),
		aiToggle("aware_playlog", "AWARE PLAY LOG: "+onOff(m.cfg.DeviceContext.PlayLog), m.cfg.DeviceContext.PlayLog),
		aiToggle("aware_system", "AWARE SYSTEM: "+onOff(m.cfg.DeviceContext.System), m.cfg.DeviceContext.System),
		aiToggle("aware_achievements", "AWARE ACHIEVEMENTS: "+onOff(m.cfg.DeviceContext.Achievements), m.cfg.DeviceContext.Achievements),
		aiCycle("library_detail", "LIBRARY DETAIL: "+strings.ToUpper(m.cfg.LibraryDetail)),
		aiCycle("request_timeout", fmt.Sprintf("TIMEOUT: %d%s", m.cfg.RequestTimeout, secondsUnit)),
		aiCycle("proactive_talk", "PROACTIVE TALK: "+strings.ToUpper(m.cfg.ProactiveTalk)),
		aiToggle("wake_word", "WAKE WORD: "+onOff(m.cfg.WakeWordEnabled), m.cfg.WakeWordEnabled),
		// CONTINUED CONVO only applies when the wake word is on; hide it otherwise.
		{OverlayItem{Code: "continued_convo", Label: "CONTINUED CONVO: " + strings.ToUpper(m.cfg.ContinuedConversation), Selected: true, Hidden: !isAI || !m.cfg.WakeWordEnabled, Indent: true}, isAI && m.cfg.WakeWordEnabled},
		{OverlayItem{Code: "mod", Label: "MOD: " + m.modLabel(), Selected: true}, true},
		// Blank separator setting the destructive Restore Defaults apart.
		{OverlayItem{Code: "spacer", Spacer: true}, false},
		{OverlayItem{Code: "restore_defaults", Label: "RESTORE DEFAULTS"}, true},
		{OverlayItem{Code: "about", Label: "ABOUT"}, true},
	}
}

// Move advances the focus by delta, skipping non-navigable slots (hidden rows,
// read-only status rows, and the separator), wrapping at both ends.
func (m *SettingsMenu) Move(delta int) {
	if m == nil {
		return
	}
	const count = settingsSlotCount
	step := 1
	if delta < 0 {
		step = -1
	}
	m.focus = ((m.focus+delta)%count + count) % count
	for m.shouldSkip(m.focus) {
		m.focus = (m.focus + step + count) % count
	}
}

func (m *SettingsMenu) shouldSkip(idx int) bool {
	slots := m.slots()
	if idx < 0 || idx >= len(slots) {
		return true
	}
	return !slots[idx].navigable
}

func (m *SettingsMenu) ToggleFocused() error {
	if m == nil {
		return fmt.Errorf("nil settings menu")
	}
	switch m.focus {
	case 0:
		next := nextInOrder(strings.ToLower(strings.TrimSpace(m.cfg.LogLevel)), logLevelOrder)
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
	case 3:
		m.cfg.STT.Cycle(1)
	case 4:
		m.cfg.Chat.Cycle(1)
	case 5:
		m.cfg.TTS.Cycle(1)
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
		m.cfg.ProactiveTalk = nextInOrder(strings.ToLower(strings.TrimSpace(m.cfg.ProactiveTalk)), config.SupportedProactiveTalkLevels())
	case 15:
		m.toggleWakeWord()
	case 16:
		m.cfg.ContinuedConversation = nextInOrder(m.cfg.ContinuedConversation,
			[]string{config.ContinuedConvoOff, config.ContinuedConvoShort, config.ContinuedConvoLong})
	case 17:
		m.cycleMod()
	case 19:
		if m.onRestore != nil {
			return m.onRestore()
		}
	case 20:
		if m.about != nil {
			m.aboutActive = true
		}
	default:
		return fmt.Errorf("unsupported focus %d", m.focus)
	}
	return nil
}

// toggleWakeWord flips the wake-word detector on/off and keeps InputTrigger in
// sync (wake_word when on, ptt when off) so there is a single source of truth.
func (m *SettingsMenu) toggleWakeWord() {
	m.cfg.WakeWordEnabled = !m.cfg.WakeWordEnabled
	if !m.cfg.WakeWordEnabled {
		m.cfg.InputTrigger = config.InputTriggerPTT
		return
	}
	m.cfg.InputTrigger = config.InputTriggerWakeWord
	if m.cfg.ContinuedConversation == "" {
		m.cfg.ContinuedConversation = config.ContinuedConvoOff
	}
}

// nextInOrder returns the entry after curr in order, wrapping around. If curr
// is absent (or order is empty) it returns the first entry.
func nextInOrder(curr string, order []string) string {
	for i, v := range order {
		if v == curr {
			return order[(i+1)%len(order)]
		}
	}
	if len(order) > 0 {
		return order[0]
	}
	return curr
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
		// Arrows adjust values only; pure action rows (ABOUT, RESTORE DEFAULTS)
		// are activated by the A button (ToggleFocused), never by cycling.
		if m.isActionRow() {
			return nil
		}
		return m.ToggleFocused()
	}
}

// isActionRow reports whether the focused row is a pure action (it does
// something when activated rather than holding a cyclable value).
func (m *SettingsMenu) isActionRow() bool {
	slots := m.slots()
	if m.focus < 0 || m.focus >= len(slots) {
		return false
	}
	switch slots[m.focus].item.Code {
	case "about", "restore_defaults":
		return true
	default:
		return false
	}
}

func (m *SettingsMenu) Overlay() OverlayState {
	if m.AboutActive() {
		return OverlayState{
			Visible: true,
			Title:   m.title,
			Footer:  "PRESS ANY BUTTON TO RETURN",
			About:   m.about,
		}
	}
	slots := m.slots()
	items := make([]OverlayItem, len(slots))
	// FocusIndex is the index into the VISIBLE (non-Hidden) row list so the
	// renderer's scroll viewport stays correct when AI-only and debug-only rows
	// are hidden.
	focusVisible := 0
	visible := 0
	for i, s := range slots {
		it := s.item
		it.Focused = s.navigable && i == m.focus
		items[i] = it
		if it.Hidden {
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
