# BMO UX Overhaul Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Redesign BMO's face to match the pixel-measured Adventure Time reference, simplify hardware buttons to three actions, fix settings persistence, remove the PTT setup screen, and add a live-updating log-level setting.

**Architecture:** Six self-contained tasks, each producing a passing test suite and a commit. Tasks 1–5 are pure logic; Task 6 is the visual redesign. Run `CGO_ENABLED=0 go build ./cmd/bmo-pak/` after Tasks 4–6 to confirm the framebuffer binary compiles.

**Tech Stack:** Go 1.22, no CGO, Linux evdev, direct framebuffer rendering. Tests: standard `go test ./...`.

---

## File map

| File | Change |
|------|--------|
| `internal/observability/logger.go` | Add `SetLevel` method |
| `internal/observability/logger_test.go` | Test `SetLevel` |
| `internal/config/config.go` | Default PTT = `BTN_SOUTH`; `Default()` sets `SetupComplete: true` |
| `internal/config/config_test.go` | Update default-PTT assertions |
| `internal/ui/settings_menu.go` | 5 items (log level + mode + 3 keys); fix mode toggle; live callback |
| `internal/ui/settings_menu_test.go` | Cover log-level cycling and mode toggle |
| `internal/ui/screen_setup.go` | `InitialScreen` always returns `ScreenMain` |
| `internal/ui/screen_setup_test.go` | Update setup-flow test |
| `internal/ui/menu.go` | Delete `PTTMenu`, `NewSetupMenu`, `NewPTTMenu` |
| `internal/input/nav.go` | Remove `NavConfirm`, `NavAISetup`, `NavPTTSetup`, `NavSettings`; unmap BTN_SOUTH |
| `internal/input/nav_test.go` | Remove deleted-constant assertions |
| `cmd/bmo-pak/main_fb.go` | New handleNav; wire logger callback to SettingsMenu |
| `cmd/bmo-pak/main_sdl.go` | Remove pttMenu; wire logger callback |
| `internal/renderer/bmo_fb.go` | New face drawing (proportional, pixel-measured palette) |
| `internal/renderer/bmo_test.go` | Update `TestStyleForExpression` for new mouth constants |

---

## Task 1 — Logger.SetLevel for live log-level changes

**Files:**
- Modify: `internal/observability/logger.go`
- Modify: `internal/observability/logger_test.go`

- [ ] **Step 1: Write the failing test**

Add to `logger_test.go` after the existing tests:

```go
func TestSetLevel(t *testing.T) {
	var buf strings.Builder
	l, err := NewLogger(filepath.Join(t.TempDir(), "test.log"), LevelInfo, &buf)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer l.Close()

	l.Debugf("should be hidden")
	if strings.Contains(buf.String(), "should be hidden") {
		t.Fatal("debug message printed at info level")
	}

	l.SetLevel(LevelDebug)
	l.Debugf("should be visible")
	if !strings.Contains(buf.String(), "should be visible") {
		t.Fatal("debug message not printed after SetLevel(LevelDebug)")
	}
}
```

- [ ] **Step 2: Run to confirm it fails**

```bash
go test ./internal/observability/ -run TestSetLevel -v
```
Expected: `FAIL — l.SetLevel undefined`

- [ ] **Step 3: Add SetLevel to logger.go**

Add after `RegisterSecret`:

```go
func (l *Logger) SetLevel(level Level) {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.level = level
	l.mu.Unlock()
}
```

- [ ] **Step 4: Run to confirm it passes**

```bash
go test ./internal/observability/ -v
```
Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/observability/logger.go internal/observability/logger_test.go
git commit -m "feat: add SetLevel to Logger for live log-level updates"
```

---

## Task 2 — Config defaults and setup-flow removal

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `internal/ui/screen_setup.go`
- Modify: `internal/ui/screen_setup_test.go`

- [ ] **Step 1: Write the failing tests**

In `config_test.go`, find or add:
```go
func TestDefaultPTTButtonIsA(t *testing.T) {
	cfg := Default()
	if len(cfg.PTTButtons) != 1 || cfg.PTTButtons[0] != "BTN_SOUTH" {
		t.Fatalf("expected PTTButtons=[BTN_SOUTH], got %v", cfg.PTTButtons)
	}
}

func TestDefaultSetupComplete(t *testing.T) {
	cfg := Default()
	if !cfg.SetupComplete {
		t.Fatal("expected SetupComplete=true in default config")
	}
}
```

In `screen_setup_test.go`, find or add:
```go
func TestInitialScreenAlwaysMain(t *testing.T) {
	// Even a brand-new empty config goes straight to main.
	flow := NewSetupFlow(Config{})
	if got := flow.InitialScreen(); got != ScreenMain {
		t.Fatalf("InitialScreen = %q, want %q", got, ScreenMain)
	}
}
```

- [ ] **Step 2: Run to confirm they fail**

```bash
go test ./internal/config/ ./internal/ui/ -run "TestDefaultPTT|TestDefaultSetup|TestInitialScreenAlways" -v
```
Expected: FAIL.

- [ ] **Step 3: Update config.go**

Change `defaultPTTButtons`:
```go
var defaultPTTButtons = []string{"BTN_SOUTH"}
```

Update `Default()` to set `SetupComplete`:
```go
func Default() Config {
	return Config{
		Version:       1,
		SetupComplete: true,
		Mode:          ModeIdle,
		InputTrigger:  InputTriggerPTT,
		PTTButtons:    DefaultPTTButtons(),
		LogLevel:      DefaultLogLevel,
		Personality:   DefaultPersonality,
	}
}
```

- [ ] **Step 4: Update screen_setup.go — InitialScreen always returns ScreenMain**

Replace the body of `InitialScreen`:
```go
func (f *SetupFlow) InitialScreen() ScreenID {
	return ScreenMain
}
```

- [ ] **Step 5: Run to confirm all tests pass**

```bash
go test ./internal/config/ ./internal/ui/ -v
```
Expected: all PASS. (Some existing setup-flow tests may need updating — fix any that assert `ScreenSetup` is returned, since that path is now gone.)

- [ ] **Step 6: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go \
        internal/ui/screen_setup.go internal/ui/screen_setup_test.go
git commit -m "fix: default PTT to A button and remove setup-flow gating"
```

---

## Task 3 — SettingsMenu: log level item + mode toggle fix + live callback

**Files:**
- Modify: `internal/ui/settings_menu.go`
- Modify: `internal/ui/settings_menu_test.go`

The menu currently has 4 items (mode, stt_key, chat_key, tts_key). We add a log-level item at position 0, shift the rest down, fix the mode toggle, and wire a live callback.

- [ ] **Step 1: Write failing tests**

Add to `settings_menu_test.go`:

```go
func TestSettingsMenuLogLevelCycles(t *testing.T) {
	cfg := config.Default() // LogLevel = "info"
	m := NewSettingsMenu(cfg)

	var gotLevel string
	m.SetLogLevelCallback(func(l string) { gotLevel = l })

	// Focus 0 = log level; cycle info → warn
	m.ToggleFocused()
	if m.Config().LogLevel != "warn" {
		t.Fatalf("expected warn, got %s", m.Config().LogLevel)
	}
	if gotLevel != "warn" {
		t.Fatalf("callback not called: gotLevel=%s", gotLevel)
	}

	// Cycle warn → error
	m.ToggleFocused()
	if m.Config().LogLevel != "error" {
		t.Fatalf("expected error, got %s", m.Config().LogLevel)
	}
}

func TestSettingsMenuModeToggles(t *testing.T) {
	cfg := config.Default() // Mode = "idle"
	m := NewSettingsMenu(cfg)
	m.Move(1) // focus = 1 (mode item)

	m.ToggleFocused() // idle → ai
	if m.Config().Mode != config.ModeAI {
		t.Fatalf("expected ai, got %s", m.Config().Mode)
	}

	m.ToggleFocused() // ai → idle
	if m.Config().Mode != config.ModeIdle {
		t.Fatalf("expected idle, got %s", m.Config().Mode)
	}
}
```

- [ ] **Step 2: Run to confirm they fail**

```bash
go test ./internal/ui/ -run "TestSettingsMenuLog|TestSettingsMenuMode" -v
```
Expected: FAIL.

- [ ] **Step 3: Replace settings_menu.go with the updated implementation**

Replace the entire file:

```go
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
// log level item is cycled. Use this to update the running logger without
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
	subtitle := []string{"UP/DOWN: NAVIGATE", "LEFT/RIGHT OR A: CYCLE VALUE"}
	footer := "START TO SAVE  B TO CLOSE"
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
```

- [ ] **Step 4: Run to confirm tests pass**

```bash
go test ./internal/ui/ -v
```
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/settings_menu.go internal/ui/settings_menu_test.go
git commit -m "feat: add log-level cycling and fix mode toggle in settings menu"
```

---

## Task 4 — Remove PTT setup screen

**Files:**
- Modify: `internal/ui/menu.go` — delete PTTMenu, NewSetupMenu, NewPTTMenu
- Modify: `internal/input/nav.go` — remove NavPTTSetup constant
- Modify: `internal/input/nav_test.go` — remove NavPTTSetup assertion
- Modify: `cmd/bmo-pak/main_fb.go` — remove pttMenu
- Modify: `cmd/bmo-pak/main_sdl.go` — remove pttMenu

- [ ] **Step 1: Delete PTTMenu from menu.go**

In `internal/ui/menu.go`, delete everything from `type PTTMenu struct` to the end of the file (the `joinPTTButtonLabels` and `togglePTTButtonState` helpers at the bottom of `ptt_buttons.go` are kept as-is; only the struct and its methods go).

The final `menu.go` retains only:
```go
package ui

import (
	"github.com/carroarmato0/nextui-bmo/internal/config"
)

type OverlayItem struct {
	Code     string
	Label    string
	Selected bool
	Focused  bool
}

type OverlayState struct {
	Visible  bool
	Title    string
	Subtitle []string
	Items    []OverlayItem
	Footer   string
}

type Menu interface {
	Title() string
	Move(delta int)
	ToggleFocused() error
	Save() (config.Config, error)
	Overlay() OverlayState
}
```

(Remove the `input` import that PTTMenu needed.)

- [ ] **Step 2: Remove NavPTTSetup from nav.go**

In `internal/input/nav.go`, remove `NavPTTSetup` from the const block and its mapping in `navActionForKey` (BTN_WEST = 308):

```go
// Remove these two lines:
NavPTTSetup  // X/West — open PTT button overlay

// And in navActionForKey, remove:
case 308: // BTN_WEST / X
    return NavPTTSetup, true
```

Also remove `NavAISetup` and `NavSettings` at the same time (they are being removed in Task 5, but group them here since nav.go is already open):

Remove from the const block:
```
NavConfirm
NavAISetup
NavPTTSetup
NavSettings
```

Remove from `navActionForKey`:
```go
case 304: // BTN_SOUTH / A   ← remove entirely (A is now PTT only)
case 307: // BTN_NORTH / Y   ← remove (was NavAISetup)
case 308: // BTN_WEST / X    ← remove (was NavPTTSetup)
case 310: // BTN_TL          ← remove (was NavSettings)
case 311: // BTN_TR          ← remove (was NavSettings)
```

Keep: NavUp, NavDown, NavLeft, NavRight, NavCancel, NavSave, NavMenu.

Updated `nav.go` constants block:
```go
const (
	NavUp     NavAction = iota // D-pad up
	NavDown                     // D-pad down
	NavLeft                     // D-pad left
	NavRight                    // D-pad right
	NavCancel                   // B/East or Select — close overlay or exit
	NavSave                     // Start — save / open settings
	NavMenu                     // Mode/Guide — exit to NextUI
)
```

Updated `navActionForKey`:
```go
func navActionForKey(code uint16) (NavAction, bool) {
	switch code {
	case 305: // BTN_EAST / B
		return NavCancel, true
	case 314: // BTN_SELECT
		return NavCancel, true
	case 315: // BTN_START
		return NavSave, true
	case 316: // BTN_MODE / menu button
		return NavMenu, true
	case btnDpadUp:
		return NavUp, true
	case btnDpadDown:
		return NavDown, true
	case btnDpadLeft:
		return NavLeft, true
	case btnDpadRight:
		return NavRight, true
	default:
		return 0, false
	}
}
```

- [ ] **Step 3: Update nav_test.go**

Remove test cases that reference `NavPTTSetup`, `NavAISetup`, `NavSettings`, `NavConfirm`. Keep and update `TestNavActionForKey` to match the new mapping:

```go
func TestNavActionForKey(t *testing.T) {
	cases := []struct {
		code uint16
		want NavAction
		ok   bool
	}{
		{305, NavCancel, true},
		{314, NavCancel, true},
		{315, NavSave, true},
		{316, NavMenu, true},
		{btnDpadUp, NavUp, true},
		{btnDpadDown, NavDown, true},
		{btnDpadLeft, NavLeft, true},
		{btnDpadRight, NavRight, true},
		{304, 0, false}, // A button — PTT only, no nav
		{307, 0, false}, // Y — no longer mapped
		{308, 0, false}, // X — no longer mapped
		{310, 0, false}, // L shoulder — no longer mapped
		{0, 0, false},
	}
	// ... same loop body as before
}
```

- [ ] **Step 4: Remove pttMenu from main_fb.go**

In `cmd/bmo-pak/main_fb.go`:
- Remove the `pttMenu := ui.NewSetupMenu(cfg)` line
- Remove all references to `pttMenu` in `handleNav` (the `NavPTTSetup` case is already gone)
- Remove the `input` import if it's now only used by ptt_shared.go (check — it's still needed for NavReader)

- [ ] **Step 5: Remove pttMenu from main_sdl.go**

In `cmd/bmo-pak/main_sdl.go`:
- Remove `pttMenu := ui.NewSetupMenu(cfg)`
- Remove all references to `pttMenu` in event handlers (the `sdl.CONTROLLER_BUTTON_X`, `sdl.K_F2`, and `NavPTTSetup` blocks)

- [ ] **Step 6: Build and test**

```bash
CGO_ENABLED=0 go build ./cmd/bmo-pak/ && go test ./...
```
Expected: build ok, all tests PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/ui/menu.go internal/input/nav.go internal/input/nav_test.go \
        cmd/bmo-pak/main_fb.go cmd/bmo-pak/main_sdl.go
git commit -m "feat: remove PTT setup screen and simplify nav action set"
```

---

## Task 5 — Wire button mapping + logger callback in main_fb.go

**Files:**
- Modify: `cmd/bmo-pak/main_fb.go`

With the simplified nav constants, `handleNav` needs updating and the SettingsMenu needs the logger callback wired up.

- [ ] **Step 1: Replace handleNav in main_fb.go**

Replace the entire `handleNav` closure with:

```go
handleNav := func(action input.NavAction) {
    // MENU always exits to NextUI.
    if action == input.NavMenu {
        running = false
        return
    }

    // B always exits — whether menu is open or not.
    if action == input.NavCancel {
        running = false
        return
    }

    // Start opens/closes the settings overlay.
    if action == input.NavSave {
        if activeMenu != nil {
            if err := commitMenu(activeMenu); err != nil {
                logger.Warnf("menu save: %v", err)
            }
            setActiveMenu(nil)
        } else {
            setActiveMenu(settingsMenu)
        }
        return
    }

    if activeMenu == nil {
        return
    }

    // Within the overlay: up/down navigate, left/right cycle the focused item.
    switch action {
    case input.NavUp:
        activeMenu.Move(-1)
    case input.NavDown:
        activeMenu.Move(1)
    case input.NavLeft, input.NavRight:
        // Cancel any keyboard-edit state (no keyboard on hardware), then cycle.
        if ed, ok := activeMenu.(interface {
            IsEditing() bool
            CancelEdit()
        }); ok && ed.IsEditing() {
            ed.CancelEdit()
        }
        if err := activeMenu.ToggleFocused(); err != nil {
            logger.Warnf("toggle focused: %v", err)
        }
        // If ToggleFocused entered edit mode (API key), cancel immediately.
        if ed, ok := activeMenu.(interface {
            IsEditing() bool
            CancelEdit()
        }); ok && ed.IsEditing() {
            ed.CancelEdit()
        }
    }
}
```

- [ ] **Step 2: Wire the logger callback to SettingsMenu**

After `settingsMenu := ui.NewSettingsMenu(cfg)`, add:

```go
settingsMenu.SetLogLevelCallback(func(level string) {
    logger.SetLevel(observability.ParseLevel(level))
    logger.Infof("log level changed to %s", level)
})
```

This requires importing `"github.com/carroarmato0/nextui-bmo/internal/observability"` in main_fb.go.

- [ ] **Step 3: Build and test**

```bash
CGO_ENABLED=0 go build ./cmd/bmo-pak/ && go test ./...
```
Expected: build ok, all tests PASS.

- [ ] **Step 4: Commit**

```bash
git add cmd/bmo-pak/main_fb.go
git commit -m "feat: simplify hardware buttons to B=exit Start=settings and wire live log level"
```

---

## Task 6 — Face redesign (pixel-measured proportions)

**Files:**
- Modify: `internal/renderer/bmo_fb.go`
- Modify: `internal/renderer/bmo_test.go`

All proportions come from `~/.claude/skills/bmo-face/SKILL.md`. The canonical coordinate system: SVG viewport 280×210, screen interior x=22 y=20 w=202 h=155. All fractions below are from that reference.

- [ ] **Step 1: Update the renderer test first**

The test file references `mouthKind` constants. Replace `TestStyleForExpression` in `bmo_test.go`:

```go
func TestStyleForExpression(t *testing.T) {
	tests := []struct {
		expr      string
		wantMouth bmoMouthType
	}{
		{expr: "neutral", wantMouth: bmoMouthIdleSmile},
		{expr: "idle", wantMouth: bmoMouthIdleSmile},
		{expr: "speaking", wantMouth: bmoMouthOpenSpeak},
		{expr: "sleeping", wantMouth: bmoMouthIdleSmile},
		{expr: "concerned", wantMouth: bmoMouthFrown},
		{expr: "error", wantMouth: bmoMouthFrown},
		{expr: "happy", wantMouth: bmoMouthOpenLarge},
		{expr: "excited", wantMouth: bmoMouthOpenLarge},
		{expr: "listening", wantMouth: bmoMouthOpenSmall},
	}
	for _, tt := range tests {
		got := styleForExpression(tt.expr)
		if got.Mouth != tt.wantMouth {
			t.Fatalf("expression %q: mouth %v, want %v", tt.expr, got.Mouth, tt.wantMouth)
		}
	}
}
```

Run — expect FAIL (types don't exist yet):
```bash
go test ./internal/renderer/ -run TestStyleForExpression -v
```

- [ ] **Step 2: Add helper functions to bmo_fb.go**

Add after the `offsetPoints` function (around line 790):

```go
// quadBezierPoints samples a quadratic Bezier curve into discrete points.
func quadBezierPoints(p0, p1, p2 point, segments int) []point {
	if segments < 2 {
		segments = 2
	}
	pts := make([]point, 0, segments+1)
	for i := 0; i <= segments; i++ {
		t := float64(i) / float64(segments)
		u := 1 - t
		x := u*u*float64(p0.X) + 2*u*t*float64(p1.X) + t*t*float64(p2.X)
		y := u*u*float64(p0.Y) + 2*u*t*float64(p1.Y) + t*t*float64(p2.Y)
		pts = append(pts, point{X: int32(math.Round(x)), Y: int32(math.Round(y))})
	}
	return pts
}

// drawBezierThick draws a thick curve by stamping a filled circle at each sample point.
func (r *Renderer) drawBezierThick(pts []point, radius int32, c rgba) {
	if radius < 1 {
		radius = 1
	}
	for _, pt := range pts {
		r.fillCircle(pt.X, pt.Y, radius, c)
	}
}

// drawThickLine draws a filled-circle thick line between two points.
func (r *Renderer) drawThickLine(x1, y1, x2, y2, radius int32, c rgba) {
	pts := quadBezierPoints(
		point{x1, y1},
		point{(x1 + x2) / 2, (y1 + y2) / 2},
		point{x2, y2},
		12,
	)
	r.drawBezierThick(pts, radius, c)
}

func max32(a, b int32) int32 {
	if a > b {
		return a
	}
	return b
}
```

- [ ] **Step 3: Replace expressionStyle, mouthKind, and styleForExpression**

Find and replace the `expressionStyle` struct, `mouthKind` type, and `styleForExpression` function (roughly lines 230–290 in bmo_fb.go). Replace with:

```go
type bmoEyeType uint8

const (
	bmoEyeDot       bmoEyeType = iota // dot: idle, concerned, thinking
	bmoEyePill                         // narrow vertical pill: excited, speaking
	bmoEyePillLarge                    // wider pill + shine: listening
	bmoEyeArc                          // upward ∩ arc: happy/squint
	bmoEyeFlat                         // horizontal line: sleeping
)

type bmoMouthType uint8

const (
	bmoMouthIdleSmile  bmoMouthType = iota // gentle upward curve
	bmoMouthFrown                          // gentle downward curve
	bmoMouthOpenLarge                      // full open with teeth + tongue
	bmoMouthOpenSpeak                      // smaller open, animated for TTS
	bmoMouthOpenSmall                      // tiny 'o': listening
)

type bmoBrowType uint8

const (
	bmoBrowNone        bmoBrowType = iota
	bmoBrowWorried                          // inner corners lower
	bmoBrowRaisedRight                      // one raised brow: thinking
)

type expressionStyle struct {
	Eye          bmoEyeType
	Mouth        bmoMouthType
	Brow         bmoBrowType
	Animated     bool // speaking mouth oscillation
	Sleepy       bool // ZZZ marks
	RightEyeUp   bool // thinking: right eye slightly higher
}

func styleForExpression(expr string) expressionStyle {
	switch normalizeExpression(expr) {
	case "listening":
		return expressionStyle{Eye: bmoEyePillLarge, Mouth: bmoMouthOpenSmall}
	case "thinking":
		return expressionStyle{Eye: bmoEyeDot, Mouth: bmoMouthIdleSmile, Brow: bmoBrowRaisedRight, RightEyeUp: true}
	case "speaking":
		return expressionStyle{Eye: bmoEyePill, Mouth: bmoMouthOpenSpeak, Animated: true}
	case "sleeping":
		return expressionStyle{Eye: bmoEyeFlat, Mouth: bmoMouthIdleSmile, Sleepy: true}
	case "concerned":
		return expressionStyle{Eye: bmoEyeDot, Mouth: bmoMouthFrown, Brow: bmoBrowWorried}
	case "smile", "laugh", "excited":
		return expressionStyle{Eye: bmoEyeArc, Mouth: bmoMouthOpenLarge}
	case "blink":
		return expressionStyle{Eye: bmoEyeFlat, Mouth: bmoMouthIdleSmile}
	default: // neutral, idle
		return expressionStyle{Eye: bmoEyeDot, Mouth: bmoMouthIdleSmile}
	}
}
```

- [ ] **Step 4: Run test — should pass now**

```bash
go test ./internal/renderer/ -run TestStyleForExpression -v
```
Expected: PASS.

- [ ] **Step 5: Replace drawBackdrop and drawFace**

Find `func (r *Renderer) drawBackdrop` and `func (r *Renderer) drawFace` and replace both functions with the following. Keep `drawCornerClock`, `drawSleepMarks`, `drawSleepCap`, `drawZ` unchanged.

```go
func (r *Renderer) drawBackdrop(layout Layout, phase float64) {
	// Simple animated highlight wash — no pupil/eye logic here any more.
	w, h := layout.W, layout.H
	r.fillRectColor(0, 0, w, h, rgba{0x4e, 0xcb, 0xa8, 255}) // body teal #4ECBA8
	for i := int32(0); i < 3; i++ {
		sx := w/5 + i*w/4
		sy := h/5 + int32(math.Sin(phase*0.7+float64(i))*float64(h)/16)
		sz := clampInt32(w/18, 18, 44)
		r.fillCircle(txClamp(sx, sz, w), txClamp(sy, sz, h), sz/2, rgba{255, 255, 255, 8})
	}
}

func (r *Renderer) drawFace(layout Layout, style expressionStyle, frame FrameState, phase float64) {
	outer := rectInset(layout.W, layout.H, layout.Margin)
	inner := rectInset(layout.W, layout.H, layout.Margin+layout.ScreenInset)

	// Body (bright teal) and screen background (pale mint).
	r.fillRoundedRect(outer.X, outer.Y, outer.W, outer.H, layout.CornerRadius,
		rgba{0x4e, 0xcb, 0xa8, 255}) // #4ECBA8
	r.fillRoundedRect(inner.X, inner.Y, inner.W, inner.H,
		layout.CornerRadius-layout.ScreenInset/2,
		rgba{0x90, 0xe5, 0xc8, 255}) // #90e5c8

	iw := float64(inner.W)
	ih := float64(inner.H)
	ix := inner.X
	iy := inner.Y

	// ── Canonical eye positions (from bmo-face skill) ──────────────
	// left cx = 20.3%, right cx = 79.2%, cy = 37.4%
	lx := ix + int32(iw*0.203)
	rx := ix + int32(iw*0.792)
	ey := iy + int32(ih*0.374)
	if style.RightEyeUp {
		ey = iy + int32(ih*0.400) // left stays normal
		// draw right eye higher below, left at default
	}

	dark := rgba{0x1a, 0x1a, 0x1a, 255}

	// ── Eyes ──────────────────────────────────────────────────────
	switch style.Eye {
	case bmoEyeDot:
		dotR := max32(4, int32(iw*0.032))
		r.fillCircle(lx, ey, dotR, dark)
		if style.RightEyeUp {
			r.fillCircle(rx, iy+int32(ih*0.348), dotR, dark) // right eye slightly higher
		} else {
			r.fillCircle(rx, ey, dotR, dark)
		}

	case bmoEyePill:
		pw := max32(5, int32(iw*0.035))
		ph := max32(14, int32(ih*0.129))
		r.fillRoundedRect(lx-pw/2, ey-ph/2, pw, ph, pw/2, dark)
		r.fillRoundedRect(rx-pw/2, ey-ph/2, pw, ph, pw/2, dark)

	case bmoEyePillLarge:
		pw := max32(8, int32(iw*0.059))
		ph := max32(18, int32(ih*0.181))
		r.fillRoundedRect(lx-pw/2, ey-ph/2, pw, ph, pw/2, dark)
		r.fillRoundedRect(rx-pw/2, ey-ph/2, pw, ph, pw/2, dark)
		shR := max32(2, int32(iw*0.015))
		r.fillCircle(lx-pw/4, ey-ph/4, shR, rgba{255, 255, 255, 140})
		r.fillCircle(rx-pw/4, ey-ph/4, shR, rgba{255, 255, 255, 140})

	case bmoEyeArc:
		// ∩ upward arc: endpoints at 41.9% y, control at 32.3% y
		// half-width = 18.8% of screen width
		ahw := int32(iw * 0.188)
		aey := iy + int32(ih*0.419)
		aqy := iy + int32(ih*0.323)
		thk := max32(3, int32(iw*0.025))
		lArc := quadBezierPoints(point{lx - ahw, aey}, point{lx, aqy}, point{lx + ahw, aey}, 14)
		rArc := quadBezierPoints(point{rx - ahw, aey}, point{rx, aqy}, point{rx + ahw, aey}, 14)
		r.drawBezierThick(lArc, thk, dark)
		r.drawBezierThick(rArc, thk, dark)

	case bmoEyeFlat:
		fhw := max32(10, int32(iw*0.074))
		fh := max32(3, int32(ih*0.032))
		r.fillRectColor(lx-fhw, ey-fh/2, fhw*2, fh, dark)
		r.fillRectColor(rx-fhw, ey-fh/2, fhw*2, fh, dark)
	}

	// ── Brows ──────────────────────────────────────────────────────
	browR := max32(2, int32(ih*0.016))
	switch style.Brow {
	case bmoBrowWorried:
		lox := ix + int32(iw*0.109)
		lix := ix + int32(iw*0.287)
		rix := ix + int32(iw*0.713)
		rox := ix + int32(iw*0.891)
		byOuter := iy + int32(ih*0.226)
		byInner := iy + int32(ih*0.323)
		r.drawThickLine(lox, byOuter, lix, byInner, browR, dark)
		r.drawThickLine(rix, byInner, rox, byOuter, browR, dark)
	case bmoBrowRaisedRight:
		rix := ix + int32(iw*0.713)
		rox := ix + int32(iw*0.891)
		byRaised := iy + int32(ih*0.194)
		byBase := iy + int32(ih*0.258)
		r.drawThickLine(rix, byBase, rox, byRaised, browR, dark)
	}

	// ── Mouth ──────────────────────────────────────────────────────
	cx := ix + inner.W/2

	// Smile/frown helpers — shared bezier control points.
	slx := ix + int32(iw*0.381) // smile left x  (38.1%)
	srx := ix + int32(iw*0.600) // smile right x (60.0%)
	sey := iy + int32(ih*0.587) // endpoints y   (58.7%)
	sqy := iy + int32(ih*0.665) // control y for smile (curves down)
	fqy := iy + int32(ih*0.510) // control y for frown (curves up)
	mouthSW := max32(3, int32(ih*0.026))

	// Open-mouth shared dims.
	mx := ix + int32(iw*0.292)   // mouth left x
	mw := int32(iw * 0.416)      // mouth width
	mty := iy + int32(ih*0.523)  // mouth top y
	mh := int32(ih * 0.277)      // mouth height
	mr := int32(float64(mh) * 0.48)
	tth := int32(float64(mh) * 0.28) // teeth height
	teeth := rgba{0xe4, 0xe4, 0xe4, 255}
	interior := rgba{0x1a, 0x78, 0x48, 255}
	tongue := rgba{0x16, 0xae, 0x81, 255}
	trx := int32(float64(mw/2) * 0.69)
	try := int32(float64(mh) * 0.16)
	tcy := mty + tth + int32(float64(mh-tth)*0.67)

	switch style.Mouth {
	case bmoMouthIdleSmile:
		smilePts := quadBezierPoints(point{slx, sey}, point{cx, sqy}, point{srx, sey}, 14)
		r.drawBezierThick(smilePts, mouthSW, dark)

	case bmoMouthFrown:
		frownPts := quadBezierPoints(point{slx, sey}, point{cx, fqy}, point{srx, sey}, 14)
		r.drawBezierThick(frownPts, mouthSW, dark)

	case bmoMouthOpenLarge:
		r.fillRoundedRect(mx, mty, mw, mh, mr, dark)
		r.fillRectColor(mx+mr/2, mty+3, mw-mr, tth-3, teeth)
		r.fillRoundedRect(mx+3, mty+tth, mw-6, mh-tth-3, mr-2, interior)
		r.fillEllipse(cx-trx, tcy-try, trx*2, try*2, tongue)

	case bmoMouthOpenSpeak:
		// Animated: mouth height oscillates with phase (8 Hz).
		smx := ix + int32(iw*0.341)
		smw := int32(iw * 0.318)
		smty := iy + int32(ih*0.548)
		smhBase := int32(ih * 0.213)
		var smh int32
		if style.Animated {
			smh = int32(float64(smhBase) * (0.50 + 0.30*math.Sin(phase*8.0)))
			if smh < smhBase/4 {
				smh = smhBase / 4
			}
		} else {
			smh = smhBase
		}
		smr := int32(float64(smh) * 0.48)
		stth := int32(float64(smh) * 0.28)
		stcy := smty + stth + int32(float64(smh-stth)*0.67)
		strx := int32(float64(smw/2) * 0.69)
		stry := int32(float64(smh) * 0.16)
		r.fillRoundedRect(smx, smty, smw, smh, smr, dark)
		r.fillRectColor(smx+smr/2, smty+3, smw-smr, stth-3, teeth)
		r.fillRoundedRect(smx+3, smty+stth, smw-6, smh-stth-3, smr-2, interior)
		r.fillEllipse(cx-strx, stcy-stry, strx*2, stry*2, tongue)

	case bmoMouthOpenSmall:
		soRX := max32(8, int32(iw*0.074))
		soRY := max32(5, int32(ih*0.065))
		soCy := iy + int32(ih*0.665)
		r.fillEllipse(cx-soRX, soCy-soRY, soRX*2, soRY*2, dark)
		r.fillEllipse(cx-soRX*3/4, soCy-soRY*3/4, soRX*3/2, soRY*3/2, interior)
	}

	if style.Sleepy {
		r.drawSleepMarks(layout, phase)
	}
}
```

- [ ] **Step 6: Remove now-unused old drawing methods**

Delete these methods from bmo_fb.go (they are no longer called):
- `drawEye`
- `drawEyeClosed`
- `drawPupil`
- `drawMouthLine`
- `drawMouthWhistle`
- `drawMouthOpen`
- `drawMouthSmile`
- `drawMouthFrown`

Also remove unused `mouthKind` constants block and old `expressionStyle` struct if any remnants remain.

- [ ] **Step 7: Build and test**

```bash
CGO_ENABLED=0 go build ./cmd/bmo-pak/ && go test ./...
```
Expected: build ok, all tests PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/renderer/bmo_fb.go internal/renderer/bmo_test.go
git commit -m "feat: redesign BMO face with pixel-measured proportions from reference images"
```

---

## Final verification

- [ ] Run full test suite:
```bash
go test ./...
```

- [ ] Build the arm64 release:
```bash
./scripts/release.sh
```

- [ ] Deploy to device:
```bash
./scripts/deploy.sh
```

---

## Implementation order

Tasks 1–5 are pure logic changes and can be done in any order. Task 6 depends on nothing else and is independent. Suggested order: 1 → 2 → 3 → 4 → 5 → 6, with a build check after Task 4 and after Task 6.
