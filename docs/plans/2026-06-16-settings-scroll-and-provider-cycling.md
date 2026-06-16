# Settings Scrolling & Provider Cycling Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Workstream 3 of the talking-animation/logging/settings design. Introduce a multi-provider `ProviderSet` config model (with an active selection), make the Settings menu's STT/CHAT/TTS rows focusable so LEFT/RIGHT cycle the active provider when AI mode is on, and add a scrolling viewport to the renderer overlay so settings rows never overflow the panel or overlap the footer.

**Architecture:** Three layered changes kept build-green at every task. (A) `config.ProviderSet{Active, Providers}` with `Current()`, `Cycle(delta)`, `Names()`; `Config.STT/Chat/TTS` flip from `Provider` to `ProviderSet`; `Normalize`/`Validate`/`Redacted`/`Secrets` updated. (B) `SettingsMenu` gains a `Cycle(delta int) error` method (added to the `ui.Menu` interface); provider rows 3/4/5 become focusable in AI mode and cycle the active provider; the renderer loop calls `Cycle(±1)` on LEFT/RIGHT. (C) `drawOverlay` builds a visible-row window from `FocusIndex`, clips out-of-window rows, and draws ▲/▼ affordances.

**CRITICAL CONSTRAINT (user directive): NO backward compatibility / NO migration code.** Do NOT add `UnmarshalJSON` shims or legacy single-object fallbacks. Existing `config.json` files (repo defaults, test fixtures, and the on-device file) are updated MANUALLY as one-off edits — these are explicit tasks below.

**Tech Stack:** Go, SDL2 software renderer (CGO), JSON config.

---

## File Structure

| File | Change | Responsibility |
| --- | --- | --- |
| `internal/config/config.go` | modify | Add `ProviderSet` type + `Current`/`Cycle`/`Names`; flip `Config.STT/Chat/TTS` to `ProviderSet`; update `Normalize`, `Validate`, `Redacted`, `Secrets`. `Default()` already yields empty sets (zero value). |
| `internal/config/config_test.go` | modify | Update `TestRoundTrip`, `TestRedactedRemovesSecrets`, `TestLoadIgnoresRemovedPromptKeys` fixtures to the new array shape; add `TestProviderSet*` unit tests. |
| `cmd/bmo-pak/main.go` | modify | Provider consumers use `.Current()`; add `FocusIndex` to `convertOverlay`; wire `NavLeft`/`NavRight` to `activeMenu.Cycle(±1)`. |
| `cmd/generate-audio/main.go` | modify | Read `cfg.TTS.Current().Voice`. |
| `internal/ui/settings_menu.go` | modify | `SetProvider` writes into the focused kind's `ProviderSet`; `Overlay()` reads `.Current()` and sets `FocusIndex`; add `Cycle(delta int) error`; make rows 3/4/5 focusable in AI mode (`shouldSkip`). |
| `internal/ui/settings_menu_test.go` | modify | Update provider fixtures to `ProviderSet`; update focusability assertions; add LEFT/RIGHT cycle tests. |
| `internal/ui/screen_setup.go` | modify | `SelectIdleOnly`/`SetProvider`/`SetAPIKey`/`ProviderSummary` operate on `ProviderSet`; add `Cycle(delta int) error` (delegates to `ToggleFocused`) to satisfy `ui.Menu`. |
| `internal/ui/screen_setup_test.go` | modify | Update `SetProvider`/summary fixtures to `ProviderSet`. |
| `internal/ui/screen_settings.go` | modify | `SetAPIKey`/`ProviderSummary` operate on `ProviderSet`. |
| `internal/ui/menu.go` | modify | Add `Cycle(delta int) error` to the `Menu` interface; add `FocusIndex int` to `OverlayState`. |
| `internal/renderer/bmo.go` | modify | Add `FocusIndex int` to `OverlayState`; scrolling viewport + clipping + ▲/▼ in `drawOverlay`. |
| `internal/renderer/bmo_test.go` | modify | Add `TestOverlay*` viewport/clip/affordance tests. |
| device `/mnt/SDCARD/.userdata/tg5040/BMO/config.json` | manual (ADB) | One-off rewrite to the array shape. |

---

## Verified current facts (do not re-derive)

- `config.Provider{Name,Model,Voice,BaseURL,APIKey}` unchanged. Providers package constructors still take a single `Provider`; only call sites change.
- `Config.STT/Chat/TTS` are `Provider` today (json `stt`/`chat`/`tts`). `Default()` leaves them zero — AI is opt-in, idle default has no providers. **This stays true with `ProviderSet` (zero value = empty set).**
- `Normalize` is called by `Load`, `Save`, `Validate`. `Validate` in `ModeAI` calls `validateAIProvider("stt", cfg.STT)` etc.
- `Redacted` zeroes `STT/Chat/TTS.APIKey`. `Secrets` collects those three APIKeys (deduped).
- `validateAIProvider(kind string, p Provider)` requires non-empty `Name` and `Model`. **Signature unchanged** — call it with `set.Current()`.
- `SettingsMenu`: `const count = 17`; `shouldSkip` skips idx 3-6 always, and idx 1 unless log level is `debug`. `shouldSkip` reads `m.cfg` (no `isAI` field; use `m.cfg.Mode == config.ModeAI`).
- `Overlay()` builds 17 items, indices: 0 log_level, 1 log_system_prompt (Hidden unless debug), 2 mode, 3 stt_status, 4 chat_status, 5 tts_status, 6 voice_status (all 4 `Disabled:!isAI`), 7-11 aware_*, 12 library_detail, 13 request_timeout, 14 proactive_talk, 15 mod, 16 restore_defaults.
- `providerModelLabel(kind, p)` → `kind+": "+model` or `kind+": NOT SET"`. `voiceStatusLabel(p)` → `"VOICE: "+voice` or `"VOICE: NOT SET"`. `internal/ui/providers.go` also has `providerSummaryLabel(kind, Provider)` and constants `providerKindSTT/Chat/TTS`.
- Renderer loop (`cmd/bmo-pak/main.go` ~451-478): `NavUp`→`Move(-1)`, `NavDown`→`Move(1)`, `NavLeft/NavRight`→cancel-edit then `ToggleFocused()` then `commitMenu`. `activeMenu` is `ui.Menu` (main.go:127). `input.NavLeft/NavRight` already exist — no `nav.go` change.
- `convertOverlay` (main.go ~692) maps `ui.OverlayState`→`renderer.OverlayState` field-by-field.
- `drawOverlay` (`internal/renderer/bmo.go:517`) loops items with no vertical bound; Disabled rows advance `top += 22`, normal rows `top += 26`; footer drawn at `panelY+panelH-28`. Font glyphs (`internal/renderer/bmo.go` ~440-490) are a 5×7 bitmap map covering digits, `A`-`Z`, and a few symbols — **no triangle glyph**. The renderer has `drawText`, `drawLine`, `fillRectColor`, `fillRoundedRect`, `clampInt32`. ▲/▼ affordances will be drawn as small filled triangles via `fillRectColor`/`drawLine` (see Task 9 for the exact helper).
- `internal/assistant/voice_test.go` uses `fakeProvider` + raw string args to `NewVoicePipeline`; it does **not** reference `config.Provider` or `config.Config`. **No edits needed there.**

---

## Task 1 — `ProviderSet` type + methods (pure, no Config change yet)

**Files:** `internal/config/config.go`, `internal/config/config_test.go`

Add `ProviderSet` and its three methods. This task does NOT touch `Config` fields, so the build/tests stay green.

- [ ] Add failing unit tests in `internal/config/config_test.go`:

```go
func TestProviderSetCurrent(t *testing.T) {
	set := ProviderSet{
		Active: "groq",
		Providers: []Provider{
			{Name: "openai", Model: "whisper-1"},
			{Name: "groq", Model: "whisper-large-v3"},
		},
	}
	if got := set.Current().Name; got != "groq" {
		t.Fatalf("Current().Name = %q, want groq", got)
	}
	// Active names a missing provider -> first.
	set.Active = "nope"
	if got := set.Current().Name; got != "openai" {
		t.Fatalf("Current().Name = %q, want openai (first fallback)", got)
	}
	// Empty Active -> first.
	set.Active = ""
	if got := set.Current().Name; got != "openai" {
		t.Fatalf("Current().Name = %q, want openai (empty fallback)", got)
	}
	// Empty set -> zero Provider.
	empty := ProviderSet{}
	if empty.Current() != (Provider{}) {
		t.Fatalf("empty Current() = %#v, want zero Provider", empty.Current())
	}
}

func TestProviderSetCycleWrapAround(t *testing.T) {
	set := ProviderSet{
		Active: "a",
		Providers: []Provider{
			{Name: "a", Model: "m1"},
			{Name: "b", Model: "m2"},
			{Name: "c", Model: "m3"},
		},
	}
	set.Cycle(1)
	if set.Active != "b" {
		t.Fatalf("after Cycle(1) Active = %q, want b", set.Active)
	}
	set.Cycle(1)
	set.Cycle(1)
	if set.Active != "a" {
		t.Fatalf("after wrapping forward Active = %q, want a", set.Active)
	}
	set.Cycle(-1)
	if set.Active != "c" {
		t.Fatalf("after Cycle(-1) Active = %q, want c (wrap backward)", set.Active)
	}
}

func TestProviderSetCycleNoOpBelowTwo(t *testing.T) {
	one := ProviderSet{Active: "a", Providers: []Provider{{Name: "a", Model: "m"}}}
	one.Cycle(1)
	if one.Active != "a" {
		t.Fatalf("single-provider Cycle changed Active to %q", one.Active)
	}
	empty := ProviderSet{}
	empty.Cycle(1) // must not panic
	if empty.Active != "" {
		t.Fatalf("empty Cycle set Active to %q", empty.Active)
	}
}

func TestProviderSetCycleFromUnresolvedActive(t *testing.T) {
	// When Active does not name a provider, Cycle resolves from index 0.
	set := ProviderSet{
		Active: "",
		Providers: []Provider{
			{Name: "a", Model: "m1"},
			{Name: "b", Model: "m2"},
		},
	}
	set.Cycle(1)
	if set.Active != "b" {
		t.Fatalf("Cycle from unresolved Active = %q, want b", set.Active)
	}
}

func TestProviderSetNames(t *testing.T) {
	set := ProviderSet{Providers: []Provider{{Name: "a"}, {Name: "b"}, {Name: "c"}}}
	got := set.Names()
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("Names() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Names()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if n := (ProviderSet{}).Names(); len(n) != 0 {
		t.Fatalf("empty Names() = %v, want empty", n)
	}
}
```

- [ ] Run (expect FAIL/compile error — `ProviderSet` undefined): `CGO_ENABLED=1 go test ./internal/config/ -run TestProviderSet -v`
- [ ] Add the type + methods to `internal/config/config.go` immediately below the `Provider` type (after its closing `}`, before `DeviceContext`):

```go
// ProviderSet is a list of interchangeable providers for one capability
// (STT, chat, or TTS) plus the name of the active selection. The active
// provider is resolved by Current(); the user cycles it from the settings
// menu. There is intentionally no migration from the old single-object
// layout — existing config.json files are updated by hand.
type ProviderSet struct {
	Active    string     `json:"active"`
	Providers []Provider `json:"providers"`
}

// Current returns the provider whose Name matches Active. If Active is empty
// or names no provider, it falls back to the first provider. If the set is
// empty it returns a zero Provider.
func (s ProviderSet) Current() Provider {
	if len(s.Providers) == 0 {
		return Provider{}
	}
	for _, p := range s.Providers {
		if p.Name == s.Active {
			return p
		}
	}
	return s.Providers[0]
}

// Cycle moves Active forward (delta>0) or backward (delta<0) by one index,
// wrapping around. It is a no-op when there are fewer than two providers.
// If Active does not currently name a provider, cycling starts from index 0.
func (s *ProviderSet) Cycle(delta int) {
	if s == nil || len(s.Providers) < 2 {
		return
	}
	idx := 0
	for i, p := range s.Providers {
		if p.Name == s.Active {
			idx = i
			break
		}
	}
	step := 1
	if delta < 0 {
		step = -1
	}
	n := len(s.Providers)
	idx = ((idx+step)%n + n) % n
	s.Active = s.Providers[idx].Name
}

// Names returns the provider names in order.
func (s ProviderSet) Names() []string {
	out := make([]string, 0, len(s.Providers))
	for _, p := range s.Providers {
		out = append(out, p.Name)
	}
	return out
}
```

- [ ] Run (expect PASS): `CGO_ENABLED=1 go test ./internal/config/ -run TestProviderSet -v`
- [ ] Lint: `golangci-lint run ./...`
- [ ] Commit: `feat(config): add ProviderSet with Current/Cycle/Names`

---

## Task 2 — Flip `Config.STT/Chat/TTS` to `ProviderSet`; update Normalize/Validate/Redacted/Secrets + fixtures

**Files:** `internal/config/config.go`, `internal/config/config_test.go`

This is the breaking type change. Update the struct, the four methods, and the config-package test fixtures together so the package compiles and its tests pass. (Other packages still won't compile until Task 3 — that is expected; run only the config package here.)

- [ ] Update the three fields in `Config` (`internal/config/config.go`):

```go
	STT           ProviderSet   `json:"stt"`
	Chat          ProviderSet   `json:"chat"`
	TTS           ProviderSet   `json:"tts"`
```

- [ ] Extend `Normalize` — add this block at the end of the method body (before its closing `}`), after the `RequestTimeout` clamp:

```go
	normalizeProviderSet(&c.STT)
	normalizeProviderSet(&c.Chat)
	normalizeProviderSet(&c.TTS)
```

  And add the helper directly below `Normalize`:

```go
// normalizeProviderSet trims provider name/model/voice/base_url and resolves a
// default Active (the first provider's name) when Active is empty.
func normalizeProviderSet(s *ProviderSet) {
	for i := range s.Providers {
		s.Providers[i].Name = strings.TrimSpace(s.Providers[i].Name)
		s.Providers[i].Model = strings.TrimSpace(s.Providers[i].Model)
		s.Providers[i].Voice = strings.TrimSpace(s.Providers[i].Voice)
		s.Providers[i].BaseURL = strings.TrimSpace(s.Providers[i].BaseURL)
	}
	s.Active = strings.TrimSpace(s.Active)
	if s.Active == "" && len(s.Providers) > 0 {
		s.Active = s.Providers[0].Name
	}
}
```

- [ ] Update `Validate`'s `ModeAI` block to validate each set has ≥1 provider, that `Active` (when non-empty) names an existing provider, and that `Current()` passes `validateAIProvider`:

```go
	if cfg.Mode == ModeAI {
		if err := validateProviderSet("stt", cfg.STT); err != nil {
			return err
		}
		if err := validateProviderSet("chat", cfg.Chat); err != nil {
			return err
		}
		if err := validateProviderSet("tts", cfg.TTS); err != nil {
			return err
		}
	}
```

  Add the helper next to `validateAIProvider`:

```go
// validateProviderSet checks that an AI-mode provider set has at least one
// provider, that a non-empty Active names an existing provider, and that the
// resolved Current() provider has the required name/model.
func validateProviderSet(kind string, s ProviderSet) error {
	if len(s.Providers) == 0 {
		return fmt.Errorf("%w: %s has no providers", ErrInvalid, kind)
	}
	if s.Active != "" {
		found := false
		for _, p := range s.Providers {
			if p.Name == s.Active {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%w: %s active %q names no provider", ErrInvalid, kind, s.Active)
		}
	}
	return validateAIProvider(kind, s.Current())
}
```

- [ ] Update `Redacted` to zero APIKeys across all providers in all three sets:

```go
func (c Config) Redacted() Config {
	cfg := c
	cfg.STT = redactProviderSet(c.STT)
	cfg.Chat = redactProviderSet(c.Chat)
	cfg.TTS = redactProviderSet(c.TTS)
	return cfg
}

// redactProviderSet returns a deep copy of the set with every APIKey cleared.
func redactProviderSet(s ProviderSet) ProviderSet {
	out := ProviderSet{Active: s.Active}
	out.Providers = make([]Provider, len(s.Providers))
	copy(out.Providers, s.Providers)
	for i := range out.Providers {
		out.Providers[i].APIKey = ""
	}
	return out
}
```

- [ ] Update `Secrets` to collect APIKeys across all providers in all three sets (deduped):

```go
func (c Config) Secrets() []string {
	secrets := make([]string, 0, 3)
	seen := map[string]struct{}{}
	for _, set := range []ProviderSet{c.STT, c.Chat, c.TTS} {
		for _, p := range set.Providers {
			value := strings.TrimSpace(p.APIKey)
			if value == "" {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			secrets = append(secrets, value)
		}
	}
	return secrets
}
```

- [ ] `Default()` requires NO change (zero `ProviderSet{}` for all three is the desired empty default). Re-read it to confirm it does not set STT/Chat/TTS — it does not. This is the manual one-off edit for repo defaults; it is satisfied automatically.

- [ ] Update fixtures in `internal/config/config_test.go`:

  `TestRoundTrip` — replace the three single-object assignments with sets:

```go
	want.STT = ProviderSet{Active: "openai-compatible", Providers: []Provider{{Name: "openai-compatible", Model: "whisper-1", BaseURL: "https://example.invalid", APIKey: "secret-stt"}}}
	want.Chat = ProviderSet{Active: "openai-compatible", Providers: []Provider{{Name: "openai-compatible", Model: "gpt-4o-mini", APIKey: "secret-chat"}}}
	want.TTS = ProviderSet{Active: "openai-compatible", Providers: []Provider{{Name: "openai-compatible", Model: "tts-1", Voice: "alloy", APIKey: "secret-tts"}}}
```

  And the assertion line that reads the fields becomes `.Current()`-based:

```go
	if got.STT.Current().APIKey != want.STT.Current().APIKey || got.Chat.Current().APIKey != want.Chat.Current().APIKey || got.TTS.Current().APIKey != want.TTS.Current().APIKey || got.TTS.Current().Voice != want.TTS.Current().Voice || len(got.PTTButtons) != len(want.PTTButtons) || got.PTTButtons[0] != want.PTTButtons[0] || got.PTTButtons[1] != want.PTTButtons[1] {
		t.Fatalf("provider fields lost: got %+v want %+v", got, want)
	}
```

  `TestRedactedRemovesSecrets` — set keys inside a provider and assert via `Current()`:

```go
func TestRedactedRemovesSecrets(t *testing.T) {
	cfg := Default()
	cfg.STT = ProviderSet{Active: "p", Providers: []Provider{{Name: "p", Model: "whisper-1", APIKey: "secret-stt"}}}
	cfg.Chat = ProviderSet{Active: "p", Providers: []Provider{{Name: "p", Model: "gpt-4o-mini", APIKey: "secret-chat"}}}
	cfg.TTS = ProviderSet{Active: "p", Providers: []Provider{{Name: "p", Model: "tts-1", APIKey: "secret-tts"}}}

	redacted := cfg.Redacted()
	if redacted.STT.Current().APIKey != "" || redacted.Chat.Current().APIKey != "" || redacted.TTS.Current().APIKey != "" {
		t.Fatalf("Redacted() kept secrets: %+v", redacted)
	}
	// Original must be untouched (Redacted returns a copy).
	if cfg.STT.Current().APIKey != "secret-stt" {
		t.Fatalf("Redacted() mutated original: %+v", cfg.STT)
	}
}
```

  `TestLoadIgnoresRemovedPromptKeys` — the legacy JSON literal uses the OLD single-object `tts` shape. Per the no-migration directive, rewrite the literal to the new array shape and keep the intent (removed prompt keys ignored):

```go
	legacy := `{"version":1,"mode":"ai","system_prompt":"old persona","tts":{"active":"openai-compatible","providers":[{"name":"openai-compatible","model":"gpt-4o-mini-tts","voice":"nova","instructions":"old instructions"}]},"log_level":"info","personality":"bmo","reduced_motion":false}`
```

  and the trailing assertion:

```go
	if cfg.TTS.Current().Model != "gpt-4o-mini-tts" || cfg.TTS.Current().Voice != "nova" {
		t.Fatalf("legacy config fields lost: %#v", cfg.TTS)
	}
```

- [ ] Add a JSON round-trip + Validate/Normalize active-resolution test:

```go
func TestProviderSetJSONRoundTripAndValidate(t *testing.T) {
	cfg := Default()
	cfg.Mode = ModeAI
	cfg.STT = ProviderSet{Providers: []Provider{{Name: "a", Model: "whisper-1"}, {Name: "b", Model: "whisper-large"}}}
	cfg.Chat = ProviderSet{Active: "b", Providers: []Provider{{Name: "a", Model: "gpt-4o-mini"}, {Name: "b", Model: "gpt-4o"}}}
	cfg.TTS = ProviderSet{Providers: []Provider{{Name: "a", Model: "tts-1", Voice: "alloy"}}}

	// Normalize resolves empty Active to first provider.
	cfg.Normalize()
	if cfg.STT.Active != "a" {
		t.Fatalf("Normalize STT.Active = %q, want a", cfg.STT.Active)
	}
	if cfg.Chat.Active != "b" {
		t.Fatalf("Normalize Chat.Active = %q, want b (preserved)", cfg.Chat.Active)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal error = %v", err)
	}
	var back Config
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("Unmarshal error = %v", err)
	}
	back.Normalize()
	if len(back.Chat.Providers) != 2 || back.Chat.Active != "b" || back.Chat.Current().Model != "gpt-4o" {
		t.Fatalf("round-trip lost chat set: %#v", back.Chat)
	}
}

func TestValidateRejectsAIWithActiveNamingNoProvider(t *testing.T) {
	cfg := Default()
	cfg.Mode = ModeAI
	cfg.STT = ProviderSet{Active: "ghost", Providers: []Provider{{Name: "real", Model: "whisper-1"}}}
	cfg.Chat = ProviderSet{Providers: []Provider{{Name: "c", Model: "gpt-4o-mini"}}}
	cfg.TTS = ProviderSet{Providers: []Provider{{Name: "t", Model: "tts-1"}}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want error for active naming no provider")
	}
}
```

  Note `TestValidateRejectsAIWithoutProviders` already passes (empty sets fail `len==0`). `TestValidateAllowsIdleWithoutProviders` still passes (idle skips provider validation). Ensure `encoding/json` is imported in the test file (it already is for other tests; confirm).

- [ ] Run (expect PASS): `CGO_ENABLED=1 go test ./internal/config/ -v`
- [ ] Lint: `golangci-lint run ./...`
- [ ] Commit: `feat(config): make STT/Chat/TTS ProviderSets (no migration)`

---

## Task 3 — Update consumers so the whole module compiles + their tests pass

**Files:** `cmd/bmo-pak/main.go`, `cmd/generate-audio/main.go`, `internal/ui/screen_setup.go`, `internal/ui/screen_settings.go`, `internal/ui/settings_menu.go`, `internal/ui/screen_setup_test.go`, `internal/ui/settings_menu_test.go`

After Task 2 the module does not compile. Fix every `cfg.STT`/`cfg.Chat`/`cfg.TTS` field read/write to go through `.Current()` (reads) or the `Providers` slice (writes). Keep behavior identical for now (focusability/cycling/scrolling come in later tasks).

- [ ] `cmd/bmo-pak/main.go` ~230-233 — replace the provider client/pipeline construction:

```go
			sttP := cfg.STT.Current()
			chatP := cfg.Chat.Current()
			ttsP := cfg.TTS.Current()
			sttClient := providers.NewOpenAICompatibleClient(providers.Config{BaseURL: sttP.BaseURL, APIKey: sttP.APIKey}, http.DefaultClient)
			chatClient := providers.NewOpenAICompatibleClient(providers.Config{BaseURL: chatP.BaseURL, APIKey: chatP.APIKey}, http.DefaultClient)
			ttsClient := providers.NewOpenAICompatibleClient(providers.Config{BaseURL: ttsP.BaseURL, APIKey: ttsP.APIKey}, http.DefaultClient)
			audioPipeline = assistant.NewVoicePipeline(machine, audioRouter, sttClient, chatClient, ttsClient, sttP.Model, chatP.Model, ttsP.Model, ttsP.Voice, personaPrompt, audioCfg.SampleRate, audioCfg.Channels, audioCfg.PlaybackChannels)
```

- [ ] `cmd/generate-audio/main.go` ~53-54 — replace:

```go
		if err == nil && strings.TrimSpace(cfg.TTS.Current().Voice) != "" {
			*voice = cfg.TTS.Current().Voice
		}
```

- [ ] `internal/ui/screen_setup.go`:
  - `SelectIdleOnly`: replace the three `s.cfg.STT = config.Provider{}` lines with `s.cfg.STT = config.ProviderSet{}`, `s.cfg.Chat = config.ProviderSet{}`, `s.cfg.TTS = config.ProviderSet{}`.
  - `SetProvider(kind, provider)`: write a single-provider set with Active set to that provider's name:

```go
func (s *SetupScreen) SetProvider(kind string, provider config.Provider) error {
	if s == nil {
		return fmt.Errorf("nil setup screen")
	}
	set := config.ProviderSet{Active: provider.Name, Providers: []config.Provider{provider}}
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case providerKindSTT:
		s.cfg.STT = set
	case providerKindChat:
		s.cfg.Chat = set
	case providerKindTTS:
		s.cfg.TTS = set
	default:
		return fmt.Errorf("unknown provider kind %q", kind)
	}
	return nil
}
```

  (Match the existing method's signature/error style — re-read the current body and preserve nil-guard and return type.)
  - `SetAPIKey`: write into the active provider of the set:

```go
	case providerKindSTT:
		setActiveAPIKey(&s.cfg.STT, key)
	case providerKindChat:
		setActiveAPIKey(&s.cfg.Chat, key)
	case providerKindTTS:
		setActiveAPIKey(&s.cfg.TTS, key)
```

  Add a shared helper in `internal/ui/providers.go`:

```go
// setActiveAPIKey writes key into the active provider of the set (or the first
// provider when Active is unresolved). No-op on an empty set.
func setActiveAPIKey(s *config.ProviderSet, key string) {
	if len(s.Providers) == 0 {
		return
	}
	idx := 0
	for i, p := range s.Providers {
		if p.Name == s.Active {
			idx = i
			break
		}
	}
	s.Providers[idx].APIKey = key
}
```

  - `ProviderSummary`: pass `.Current()` to `providerSummaryLabel`:

```go
	case providerKindSTT:
		return providerSummaryLabel("STT", s.cfg.STT.Current())
	case providerKindChat:
		return providerSummaryLabel("CHAT", s.cfg.Chat.Current())
	case providerKindTTS:
		return providerSummaryLabel("TTS", s.cfg.TTS.Current())
```

- [ ] `internal/ui/screen_settings.go` — `SetAPIKey` cases and `ProviderSummary` cases: identical treatment to `screen_setup.go` above (use `setActiveAPIKey(&s.cfg.STT, key)` etc.; `providerSummaryLabel("STT", s.cfg.STT.Current())` etc.).

- [ ] `internal/ui/settings_menu.go` — for THIS task only make it compile (read-side):
  - `Overlay()` provider rows: `providerModelLabel("STT", m.cfg.STT.Current())`, `providerModelLabel("CHAT", m.cfg.Chat.Current())`, `providerModelLabel("TTS", m.cfg.TTS.Current())`, `voiceStatusLabel(m.cfg.TTS.Current())`.
  - `SetProvider(kind, provider)`: write a single-provider set (same pattern as setup screen):

```go
	set := config.ProviderSet{Active: provider.Name, Providers: []config.Provider{provider}}
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case providerKindSTT:
		m.cfg.STT = set
	case providerKindChat:
		m.cfg.Chat = set
	case providerKindTTS:
		m.cfg.TTS = set
	}
```

- [ ] `internal/ui/screen_setup_test.go` ~62-64 — `SetProvider` calls still pass `config.Provider{...}` and are fine (SetProvider wraps them). No change needed unless an assertion reads `.STT` directly — re-read; if `saved.STT` is read as a `Provider`, change to `saved.STT.Current()`.

- [ ] `internal/ui/settings_menu_test.go` ~286-307 — update the two fixtures that assign `cfg.STT = config.Provider{...}`:

```go
	cfg.STT = config.ProviderSet{Active: "openai-compatible", Providers: []config.Provider{{Name: "openai-compatible", Model: "whisper-1", APIKey: "sk-s"}}}
	cfg.Chat = config.ProviderSet{Active: "openai-compatible", Providers: []config.Provider{{Name: "openai-compatible", Model: "gpt-4o-mini"}}}
	cfg.TTS = config.ProviderSet{Active: "openai-compatible", Providers: []config.Provider{{Name: "openai-compatible", Model: "tts-1", Voice: "nova", APIKey: "sk-t"}}}
```

  and:

```go
	cfg.TTS = config.ProviderSet{Active: "openai-compatible", Providers: []config.Provider{{Name: "openai-compatible", Model: "tts-1"}}}
```

  The label assertions (`STT: whisper-1`, `CHAT: gpt-4o-mini`, `TTS: tts-1`, `VOICE: nova`, `VOICE: NOT SET`) stay valid because `providerModelLabel` now reads `.Current()`.

- [ ] Run (expect PASS, full module compiles): `CGO_ENABLED=1 go test ./...`
- [ ] Lint: `golangci-lint run ./...`
- [ ] Commit: `refactor: route provider consumers through ProviderSet.Current()`

---

## Task 4 — Settings menu: focusable provider rows + `Cycle(delta)` + Overlay labels

**Files:** `internal/ui/settings_menu.go`, `internal/ui/settings_menu_test.go`

Make rows 3/4/5 (stt/chat/tts) focusable in AI mode and cycle the active provider. Row 6 (voice) stays a non-focusable status row reflecting `TTS.Current().Voice`.

- [ ] Add failing tests in `internal/ui/settings_menu_test.go`:

```go
func TestSettingsMenuProviderRowsFocusableWhenAI(t *testing.T) {
	cfg := config.Default()
	cfg.Mode = config.ModeAI
	cfg.STT = config.ProviderSet{Active: "a", Providers: []config.Provider{{Name: "a", Model: "whisper-1"}, {Name: "b", Model: "whisper-large"}}}
	cfg.Chat = config.ProviderSet{Providers: []config.Provider{{Name: "c", Model: "gpt-4o-mini"}}}
	cfg.TTS = config.ProviderSet{Providers: []config.Provider{{Name: "t", Model: "tts-1", Voice: "nova"}}}
	m := NewSettingsMenu(cfg)

	// Move down past mode (2) should be able to land on 3,4,5 but skip 6.
	m.Move(1) // 0 -> 1 hidden? info level so 1 skipped -> 2
	// Drive focus explicitly via repeated Move and assert 3/4/5 reachable, 6 not.
	reachable := map[int]bool{}
	for i := 0; i < 30; i++ {
		reachable[m.focusIndexForTest()] = true
		m.Move(1)
	}
	for _, idx := range []int{3, 4, 5} {
		if !reachable[idx] {
			t.Errorf("row %d not focusable in AI mode", idx)
		}
	}
	if reachable[6] {
		t.Error("voice row 6 should remain non-focusable")
	}
}

func TestSettingsMenuCycleChangesActiveProvider(t *testing.T) {
	cfg := config.Default()
	cfg.Mode = config.ModeAI
	cfg.STT = config.ProviderSet{Active: "a", Providers: []config.Provider{{Name: "a", Model: "whisper-1"}, {Name: "b", Model: "whisper-large"}}}
	cfg.Chat = config.ProviderSet{Providers: []config.Provider{{Name: "c", Model: "gpt-4o-mini"}}}
	cfg.TTS = config.ProviderSet{Providers: []config.Provider{{Name: "t", Model: "tts-1"}}}
	m := NewSettingsMenu(cfg)
	m.focusForTest(3) // stt row

	if err := m.Cycle(1); err != nil {
		t.Fatalf("Cycle(1) error = %v", err)
	}
	if got := m.Config().STT.Active; got != "b" {
		t.Fatalf("after Cycle(1) STT.Active = %q, want b", got)
	}
	if err := m.Cycle(-1); err != nil {
		t.Fatalf("Cycle(-1) error = %v", err)
	}
	if got := m.Config().STT.Active; got != "a" {
		t.Fatalf("after Cycle(-1) STT.Active = %q, want a", got)
	}
	// Label reflects active provider's model.
	if got := m.Overlay().Items[3].Label; got != "STT: whisper-1" {
		t.Fatalf("stt label = %q, want STT: whisper-1", got)
	}
	m.focusForTest(3)
	_ = m.Cycle(1)
	if got := m.Overlay().Items[3].Label; got != "STT: whisper-large" {
		t.Fatalf("stt label after cycle = %q, want STT: whisper-large", got)
	}
}

func TestSettingsMenuCycleNonProviderDelegatesForward(t *testing.T) {
	// Non-provider rows ignore sign and behave like ToggleFocused (forward).
	cfg := config.Default()
	m := NewSettingsMenu(cfg)
	m.focusForTest(13) // request_timeout
	before := m.Config().RequestTimeout
	if err := m.Cycle(-1); err != nil { // negative still advances forward
		t.Fatalf("Cycle(-1) error = %v", err)
	}
	if m.Config().RequestTimeout == before {
		t.Fatalf("Cycle on timeout row did not advance (still %d)", before)
	}
}
```

  Add tiny test-only helpers (same package, so use a `_test.go`-local method on the real type) at the bottom of `settings_menu_test.go`:

```go
func (m *SettingsMenu) focusForTest(i int)    { m.focus = i }
func (m *SettingsMenu) focusIndexForTest() int { return m.focus }
```

- [ ] Update `TestSettingsMenuAIStatusEnabledWhenAI` (existing): it currently asserts rows 3,4,5,6 are NOT `Focused`. After this change 3/4/5 become focusable, but they are only `Focused` when `m.focus==idx`; a freshly constructed menu has `focus==0`, so all four are still un-focused at construction. **Keep the test as-is** (it only inspects `Overlay()` of a fresh menu, where focus is 0) — verify it still passes after implementation; if it begins to fail because the loop checks Disabled only, no change is needed. Do NOT loosen the Disabled assertion.

- [ ] Run (expect FAIL — `Cycle` undefined, helpers undefined): `CGO_ENABLED=1 go test ./internal/ui/ -run TestSettings -v`

- [ ] Implement in `internal/ui/settings_menu.go`:
  - `shouldSkip`: make rows 3/4/5 focusable only in AI mode; row 6 always skipped:

```go
func (m *SettingsMenu) shouldSkip(idx int) bool {
	isAI := m.cfg.Mode == config.ModeAI
	if idx >= 3 && idx <= 5 {
		return !isAI // focusable only in AI mode
	}
	if idx == 6 {
		return true // voice is a read-only status row
	}
	if idx == 1 && strings.ToLower(strings.TrimSpace(m.cfg.LogLevel)) != "debug" {
		return true
	}
	return false
}
```

  - Add `Cycle(delta int) error`. Provider rows cycle the active provider (honoring sign) and fire the (optional) provider-change callback; all other rows delegate to the existing forward-only `ToggleFocused`:

```go
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
```

  (No provider-change callback exists today; none is required. If `SetProvider`-style callbacks are added later they wire here.)

- [ ] Run (expect PASS): `CGO_ENABLED=1 go test ./internal/ui/ -run TestSettings -v`
- [ ] Lint: `golangci-lint run ./...`
- [ ] Commit: `feat(ui): focusable provider rows + bidirectional Cycle in settings`

---

## Task 5 — `ui.Menu` interface gains `Cycle`; wire main.go LEFT/RIGHT; `setupScreen.Cycle`

**Files:** `internal/ui/menu.go`, `internal/ui/screen_setup.go`, `cmd/bmo-pak/main.go`

- [ ] Add `Cycle(delta int) error` to the `Menu` interface in `internal/ui/menu.go`:

```go
type Menu interface {
	Title() string
	Move(delta int)
	ToggleFocused() error
	Cycle(delta int) error
	Save() (config.Config, error)
	Overlay() OverlayState
}
```

  Keep `ToggleFocused` in the interface (`SettingsMenu.Cycle` delegates to it; other implementers/tests may call it).

- [ ] Make the other `Menu` implementer (the setup screen, `internal/ui/screen_setup.go`) satisfy the interface by delegating to `ToggleFocused`:

```go
// Cycle delegates to ToggleFocused; the setup screen has no provider rows to
// cycle, so LEFT and RIGHT behave identically (forward).
func (s *SetupScreen) Cycle(delta int) error {
	return s.ToggleFocused()
}
```

  (Re-read `screen_setup.go` to confirm the receiver type implementing `Menu` is `*SetupScreen` and that it already has `ToggleFocused`. If `ToggleFocused` lives on a different setup type, attach `Cycle` to that same type.)

- [ ] Wire the renderer loop in `cmd/bmo-pak/main.go` (~451-478). Replace the single `ToggleFocused()` call in the `NavLeft, NavRight` case with direction-aware `Cycle`:

```go
		case input.NavLeft, input.NavRight:
			type editable interface {
				IsEditing() bool
				CancelEdit()
			}
			if ed, ok := activeMenu.(editable); ok && ed.IsEditing() {
				ed.CancelEdit()
			}
			delta := 1
			if action == input.NavLeft {
				delta = -1
			}
			if err := activeMenu.Cycle(delta); err != nil {
				logger.Debugf("cycle focused: %v", err)
			}
			if ed, ok := activeMenu.(editable); ok && ed.IsEditing() {
				ed.CancelEdit()
			}
			if err := commitMenu(activeMenu); err != nil {
				logger.Debugf("auto-save: %v", err)
			}
```

- [ ] Run (expect PASS): `CGO_ENABLED=1 go test ./...`
- [ ] Lint: `golangci-lint run ./...`
- [ ] Commit: `feat(ui): add Cycle to Menu interface and wire LEFT/RIGHT`

---

## Task 6 — `FocusIndex` plumbing (ui + renderer + convertOverlay)

**Files:** `internal/ui/menu.go`, `internal/ui/settings_menu.go`, `internal/renderer/bmo.go`, `cmd/bmo-pak/main.go`

Plumb the focused row index from menu to renderer so the viewport can keep it visible. **`FocusIndex` is the index into the VISIBLE (non-Hidden) row list**, not the raw item index — this avoids an off-by-one when the debug-only `log_system_prompt` row is hidden.

- [ ] Add `FocusIndex int` to `ui.OverlayState` (`internal/ui/menu.go`):

```go
type OverlayState struct {
	Visible    bool
	Title      string
	Subtitle   []string
	Items      []OverlayItem
	Footer     string
	FocusIndex int
}
```

- [ ] Add `FocusIndex int` to `renderer.OverlayState` (`internal/renderer/bmo.go`):

```go
type OverlayState struct {
	Visible    bool
	Title      string
	Subtitle   []string
	Items      []OverlayItem
	Footer     string
	FocusIndex int
}
```

- [ ] In `SettingsMenu.Overlay()` (`internal/ui/settings_menu.go`), compute the visible-row index of `m.focus` and set it. Add before the `return OverlayState{...}`:

```go
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
```

  and set `FocusIndex: focusVisible` in the returned `OverlayState`. (When the focused raw item is itself Hidden — which should not happen given `shouldSkip` — `focusVisible` stays at its last computed value, which is acceptable.)

- [ ] In `convertOverlay` (`cmd/bmo-pak/main.go` ~692), propagate the field on the returned `&renderer.OverlayState{...}`:

```go
		FocusIndex: src.FocusIndex,
```

- [ ] Run (expect PASS — no behavior change yet): `CGO_ENABLED=1 go test ./...`
- [ ] Lint: `golangci-lint run ./...`
- [ ] Commit: `feat: plumb visible FocusIndex from menu to renderer overlay`

---

## Task 7 — Renderer scrolling viewport + clipping + ▲/▼ in `drawOverlay`

**Files:** `internal/renderer/bmo.go`, `internal/renderer/bmo_test.go`

`drawOverlay` currently advances `top` for every visible item with no bound. Add a windowed render: compute a single normalized stride for visible rows, derive `maxVisibleRows` from the content height, maintain a scroll offset that keeps `FocusIndex` inside `[offset, offset+maxVisibleRows)`, render only windowed rows, and draw ▲ (offset>0) / ▼ (more below) affordances.

Because the renderer needs CGO/SDL, tests construct a headless `*Renderer` the same way existing renderer tests do. **Re-read `internal/renderer/bmo_test.go` for the existing test harness/constructor pattern (e.g. an offscreen renderer helper) and reuse it.** The new tests below assume a helper `newTestRenderer(t)` returning `*Renderer` with a backing surface; adapt names to whatever the file already provides. If no such harness exists, drive the pure geometry through an extracted helper (see `overlayWindow` below) which needs no SDL and can be unit-tested directly with `CGO_ENABLED=1` (still required because the package imports SDL).

- [ ] Add failing tests in `internal/renderer/bmo_test.go` exercising the pure window math via an exported-to-package helper:

```go
func TestOverlayWindowClampsAndKeepsFocusVisible(t *testing.T) {
	// 10 visible rows, window of 4. Focus near the end must scroll.
	off := overlayWindow(10, 4, 9)
	if off < 0 || off > 6 {
		t.Fatalf("offset %d out of clamp range [0,6]", off)
	}
	if !(9 >= off && 9 < off+4) {
		t.Fatalf("focus 9 not within window [%d,%d)", off, off+4)
	}

	// Focus at top -> offset 0.
	if off := overlayWindow(10, 4, 0); off != 0 {
		t.Fatalf("offset for focus 0 = %d, want 0", off)
	}

	// Fits entirely -> offset 0.
	if off := overlayWindow(3, 4, 2); off != 0 {
		t.Fatalf("offset when all fit = %d, want 0", off)
	}

	// Last window does not overscroll past content.
	off = overlayWindow(10, 4, 8)
	if off+4 > 10 {
		t.Fatalf("window [%d,%d) overruns 10 rows", off, off+4)
	}
}

func TestOverlayWindowDegenerate(t *testing.T) {
	if off := overlayWindow(0, 4, 0); off != 0 {
		t.Fatalf("empty content offset = %d, want 0", off)
	}
	if off := overlayWindow(5, 0, 3); off != 0 {
		t.Fatalf("zero window offset = %d, want 0", off)
	}
}
```

- [ ] Run (expect FAIL — `overlayWindow` undefined): `CGO_ENABLED=1 go test ./internal/renderer/ -run TestOverlay -v`

- [ ] Add the pure helper in `internal/renderer/bmo.go` (above `drawOverlay`):

```go
// overlayWindow returns the scroll offset (index of the first rendered visible
// row) such that focus stays inside [offset, offset+maxRows). It clamps so the
// window never runs past the content. Degenerate inputs yield 0.
func overlayWindow(total, maxRows, focus int) int {
	if total <= maxRows || maxRows <= 0 || total <= 0 {
		return 0
	}
	offset := 0
	if focus >= maxRows {
		offset = focus - maxRows + 1
	}
	maxOffset := total - maxRows
	if offset > maxOffset {
		offset = maxOffset
	}
	if offset < 0 {
		offset = 0
	}
	return offset
}
```

- [ ] Rewrite the item-drawing portion of `drawOverlay` (from `top += 18` through the footer) to window the visible rows with a single normalized stride and draw affordances. Replace lines 533-565 with:

```go
	top += 18

	const rowStride = int32(26) // normalized stride for every visible row

	// Collect visible rows (Hidden excluded) so scrolling math is stable.
	visibleIdx := make([]int, 0, len(overlay.Items))
	for i := range overlay.Items {
		if overlay.Items[i].Hidden {
			continue
		}
		visibleIdx = append(visibleIdx, i)
	}

	footerY := panelY + panelH - 28
	// Reserve a row of headroom above the footer for the ▼ affordance.
	contentBottom := footerY - rowStride
	maxRows := int((contentBottom - top) / rowStride)
	if maxRows < 1 {
		maxRows = 1
	}

	offset := overlayWindow(len(visibleIdx), maxRows, overlay.FocusIndex)
	end := offset + maxRows
	if end > len(visibleIdx) {
		end = len(visibleIdx)
	}

	// ▲ when rows exist above the window.
	if offset > 0 {
		r.drawUpTriangle(left, top-12, rgba{176, 213, 206, 255})
	}

	for _, vi := range visibleIdx[offset:end] {
		item := overlay.Items[vi]
		if item.Disabled {
			r.fillRectColor(left, top+3, 10, 10, rgba{40, 65, 70, 255})
			r.drawText(left+20, top, 2, rgba{95, 115, 115, 255}, item.Label)
			top += rowStride
			continue
		}
		boxColor := rgba{79, 139, 141, 255}
		if item.Selected {
			boxColor = rgba{170, 232, 183, 255}
		}
		if item.Focused {
			boxColor = rgba{255, 241, 145, 255}
		}
		r.fillRectColor(left, top+3, 10, 10, boxColor)
		if item.Selected {
			r.drawLine(left+2, top+8, left+4, top+11, rgba{16, 49, 56, 255})
			r.drawLine(left+4, top+11, left+8, top+3, rgba{16, 49, 56, 255})
		}
		labelColor := rgba{214, 235, 227, 255}
		if item.Focused {
			labelColor = rgba{255, 241, 145, 255}
		}
		r.drawText(left+20, top, 2, labelColor, item.Label)
		top += rowStride
	}

	// ▼ when rows exist below the window.
	if end < len(visibleIdx) {
		r.drawDownTriangle(left, top, rgba{176, 213, 206, 255})
	}

	if strings.TrimSpace(overlay.Footer) != "" {
		r.drawText(left, footerY, 2, rgba{176, 213, 206, 255}, strings.ToUpper(overlay.Footer))
	}
```

  (Note: this normalizes Disabled rows to the same 26px stride as normal rows so the window math is exact; the previous 22px-for-Disabled detail is intentionally dropped.)

- [ ] Add the two triangle helpers near `drawLine` (the font has no triangle glyph, so draw filled triangles from horizontal runs):

```go
// drawUpTriangle draws a small upward-pointing filled triangle with its apex
// near (x,y). Used as a "more content above" scroll affordance.
func (r *Renderer) drawUpTriangle(x, y int32, c rgba) {
	const h = int32(8)
	for row := int32(0); row < h; row++ {
		half := row
		r.fillRectColor(x+h-half, y+row, 2*half+1, 1, c)
	}
}

// drawDownTriangle draws a small downward-pointing filled triangle with its
// apex near (x, y+h). Used as a "more content below" scroll affordance.
func (r *Renderer) drawDownTriangle(x, y int32, c rgba) {
	const h = int32(8)
	for row := int32(0); row < h; row++ {
		half := h - 1 - row
		r.fillRectColor(x+h-half, y+row, 2*half+1, 1, c)
	}
}
```

- [ ] Add a rendering smoke test that the window never draws more than `maxRows` rows. Since `drawOverlay` writes pixels, assert via the window helper count (already covered) PLUS a guard test that a long overlay does not panic when rendered through the existing test renderer harness:

```go
func TestDrawOverlayLongListDoesNotOverflow(t *testing.T) {
	r := newTestRenderer(t) // reuse existing harness; adapt name if different
	items := make([]OverlayItem, 20)
	for i := range items {
		items[i] = OverlayItem{Code: "x", Label: "ROW", Focused: i == 19}
	}
	st := OverlayState{Visible: true, Title: "SETTINGS", Items: items, Footer: "CLOSE", FocusIndex: 19}
	// Must not panic and must clip; we assert it returns (no bounds error).
	r.drawOverlay(LayoutFor(1024, 768), st)
}
```

  If `bmo_test.go` has no `newTestRenderer` harness, drop this last test and rely on `TestOverlayWindow*` for coverage (the window math is the load-bearing logic). Confirm by re-reading the test file before writing.

- [ ] Run (expect PASS): `CGO_ENABLED=1 go test ./internal/renderer/ -run TestOverlay -v` and `CGO_ENABLED=1 go test ./internal/renderer/ -v`
- [ ] Lint: `golangci-lint run ./...`
- [ ] Commit: `feat(renderer): scrolling viewport + clip + scroll affordances in drawOverlay`

---

## Task 8 — Manual one-off config edits (no migration code)

**Files:** repo `config.Default()` (already correct), device `/mnt/SDCARD/.userdata/tg5040/BMO/config.json`

`config.Default()` was confirmed in Task 2 to leave STT/Chat/TTS as empty `ProviderSet{}` — no edit needed. All test fixtures were converted in Tasks 2-3. The only remaining one-off is the on-device config file, which uses the OLD single-object layout and would otherwise fail to populate the new array fields.

- [ ] Pull the current device config to inspect it:

```bash
adb pull /mnt/SDCARD/.userdata/tg5040/BMO/config.json /tmp/bmo-device-config.json
```

- [ ] Rewrite each of `stt`/`chat`/`tts` from the old single object to the new `{"active": <name>, "providers": [ <the old object> ]}` shape. Example transformation (old → new):

  Old:
```json
"stt": {"name": "openai-compatible", "model": "whisper-1", "base_url": "https://api.openai.com/v1", "api_key": "sk-..."}
```
  New:
```json
"stt": {"active": "openai-compatible", "providers": [{"name": "openai-compatible", "model": "whisper-1", "base_url": "https://api.openai.com/v1", "api_key": "sk-..."}]}
```
  Apply the same wrap to `chat` and `tts` (preserve `voice` on the tts provider). Edit `/tmp/bmo-device-config.json` accordingly.

- [ ] Push it back and restart the app:

```bash
adb push /tmp/bmo-device-config.json /mnt/SDCARD/.userdata/tg5040/BMO/config.json
```

  Kill the running pak so it reloads (from project memory — `pkill -f bmo-pak` is unreliable):

```bash
adb shell "kill -9 \$(adb shell 'ps | grep bmo-pak | grep -v grep | head -1 | awk \"{print \$1}\"')"
```

- [ ] Verify on device: open Settings, confirm STT/CHAT/TTS rows render the active provider's model and that LEFT/RIGHT cycle when multiple providers are present (single-provider sets are no-ops, which is expected). Tail logs if needed: `./scripts/debug-logs.sh` (device log at `/mnt/SDCARD/.userdata/tg5040/logs/BMO.txt`).

- [ ] No commit (device-only edit). If the repo ships an example config under version control, apply the same wrap to it and commit: `chore: convert example config.json to ProviderSet shape`.

---

## Task 9 — Full suite + lint gate

**Files:** none (verification)

- [ ] Full build (CGO): `CGO_ENABLED=1 go build ./...`
- [ ] Full test suite: `CGO_ENABLED=1 go test ./...`
- [ ] Race check (optional but recommended): `CGO_ENABLED=1 go test -race ./...`
- [ ] Lint clean: `golangci-lint run ./...`
- [ ] Confirm pure-Go subset still builds: `CGO_ENABLED=0 go build ./cmd/render-faces`

---

## Self-Review

Map each spec testing bullet to its covering task:

- **`ProviderSet`: `Current`/`Cycle`/`Names`, wrap-around, empty/zero, single-provider no-op, unresolved-Active resolution** → Task 1 (`TestProviderSetCurrent`, `TestProviderSetCycleWrapAround`, `TestProviderSetCycleNoOpBelowTwo`, `TestProviderSetCycleFromUnresolvedActive`, `TestProviderSetNames`).
- **`Validate`/`Normalize` with active-name resolution; empty sets rejected in AI mode; idle allowed** → Task 2 (`TestProviderSetJSONRoundTripAndValidate`, `TestValidateRejectsAIWithActiveNamingNoProvider`, existing `TestValidateRejectsAIWithoutProviders`, `TestValidateAllowsIdleWithoutProviders`).
- **JSON round-trip preserves the set** → Task 2 (`TestProviderSetJSONRoundTripAndValidate`, updated `TestRoundTrip`).
- **`Redacted`/`Secrets` across all providers in all sets** → Task 2 (`TestRedactedRemovesSecrets`, plus `Secrets` exercised by build + lint; add an explicit assertion if desired).
- **Settings: LEFT/RIGHT cycles the active provider when AI on; rows non-focusable when AI off; label reflects the active provider** → Task 4 (`TestSettingsMenuProviderRowsFocusableWhenAI`, `TestSettingsMenuCycleChangesActiveProvider`), existing `TestSettingsMenuAIStatusDisabledWhenIdle`/`...EnabledWhenAI`; non-provider rows still cycle forward (`TestSettingsMenuCycleNonProviderDelegatesForward`).
- **LEFT/RIGHT wired through the render loop** → Task 5 (`Cycle` on `ui.Menu`, main.go delta wiring, `SetupScreen.Cycle`).
- **Renderer: items beyond `maxVisibleRows` clipped; focused row stays visible; ▲/▼ drawn when content extends** → Task 7 (`TestOverlayWindowClampsAndKeepsFocusVisible`, `TestOverlayWindowDegenerate`, `TestDrawOverlayLongListDoesNotOverflow`) and `FocusIndex` plumbing in Task 6.
- **No migration code** → Tasks 2/3 (no `UnmarshalJSON`, no legacy fallback) and Task 8 (manual device edit).

**Risks / decisions to flag:**
- **Fixtures touched:** 3 test files (`internal/config/config_test.go`, `internal/ui/settings_menu_test.go`, `internal/ui/screen_setup_test.go`). `internal/assistant/voice_test.go` is NOT affected (uses `fakeProvider`, not `config.Provider`). The legacy-JSON fixture in `TestLoadIgnoresRemovedPromptKeys` is rewritten to the array shape per the no-migration rule (it stops covering old-shape configs — acceptable by directive).
- **Glyph affordance:** the bitmap font has no triangle glyph, so ▲/▼ are drawn as filled triangles via `fillRectColor` horizontal runs (`drawUpTriangle`/`drawDownTriangle`) rather than text. If a simpler `^`/`v` text marker is preferred, `drawText` covers neither caret nor lowercase reliably — the filled-triangle helper is the safe choice.
- **`FocusIndex` = visible-row index (not raw):** chosen to avoid an off-by-one when the debug-only `log_system_prompt` row is hidden. `Overlay()` computes it; the renderer windows over the visible list only.
- **Disabled-row stride normalized to 26px:** the viewport uses one stride for exact window math, dropping the old 22px-for-Disabled spacing. Visual spacing of the disabled provider/voice rows changes slightly; acceptable and simpler.
- **Renderer test harness uncertainty:** `internal/renderer/bmo_test.go` currently has no overlay tests; the pixel-level `TestDrawOverlayLongListDoesNotOverflow` depends on an existing offscreen-renderer helper. If none exists, rely on the pure `overlayWindow` tests (the load-bearing logic) and skip the pixel test. Re-read the test file before implementing Task 7.
