package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	want := Default()
	want.Mode = ModeAI
	want.InputTrigger = InputTriggerWakeWord
	want.LogLevel = "debug"
	want.STT = ProviderSet{Active: "openai-compatible", Providers: []Provider{{Name: "openai-compatible", Model: "whisper-1", BaseURL: "https://example.invalid", APIKey: "secret-stt"}}}
	want.Chat = ProviderSet{Active: "openai-compatible", Providers: []Provider{{Name: "openai-compatible", Model: "gpt-4o-mini", APIKey: "secret-chat"}}}
	want.TTS = ProviderSet{Active: "openai-compatible", Providers: []Provider{{Name: "openai-compatible", Model: "tts-1", Voice: "alloy", APIKey: "secret-tts"}}}
	want.PTTButtons = []string{"BTN_TL2", "BTN_TR2"}
	want.LogSystemPrompt = true

	if err := Save(path, want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got.Mode != want.Mode || got.InputTrigger != want.InputTrigger || got.LogLevel != want.LogLevel || got.Personality != want.Personality {
		t.Fatalf("round-trip mismatch: got %+v want %+v", got, want)
	}
	if got.STT.Current().APIKey != want.STT.Current().APIKey || got.Chat.Current().APIKey != want.Chat.Current().APIKey || got.TTS.Current().APIKey != want.TTS.Current().APIKey || got.TTS.Current().Voice != want.TTS.Current().Voice || len(got.PTTButtons) != len(want.PTTButtons) || got.PTTButtons[0] != want.PTTButtons[0] || got.PTTButtons[1] != want.PTTButtons[1] {
		t.Fatalf("provider fields lost: got %+v want %+v", got, want)
	}
	if got.LogSystemPrompt != want.LogSystemPrompt {
		t.Fatalf("LogSystemPrompt lost: got %v want %v", got.LogSystemPrompt, want.LogSystemPrompt)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestLoadMissingReturnsDefaultAndSentinel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.json")

	got, err := Load(path)
	if err != ErrNotFound {
		t.Fatalf("Load() error = %v, want ErrNotFound", err)
	}
	if got.Mode != ModeIdle || got.LogLevel != DefaultLogLevel || got.Personality != DefaultPersonality || got.InputTrigger != InputTriggerPTT {
		t.Fatalf("unexpected defaults: %+v", got)
	}
}

func TestSaveCreatesParents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "config.json")

	if err := Save(path, Default()); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config not written: %v", err)
	}
}

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

func TestValidateAllowsIdleWithoutProviders(t *testing.T) {
	cfg := Default()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsAIWithoutProviders(t *testing.T) {
	cfg := Default()
	cfg.Mode = ModeAI
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
}

func TestDefaultPTTButtonIsA(t *testing.T) {
	cfg := Default()
	if len(cfg.PTTButtons) != 1 || cfg.PTTButtons[0] != "BTN_EAST" {
		t.Fatalf("expected PTTButtons=[BTN_SOUTH], got %v", cfg.PTTButtons)
	}
}

func TestLoadIgnoresRemovedPromptKeys(t *testing.T) {
	// Configs written before prompts moved to persona.txt/voice.txt may still
	// carry system_prompt and tts.instructions; they must load cleanly.
	path := filepath.Join(t.TempDir(), "config.json")
	legacy := `{"version":1,"mode":"ai","system_prompt":"old persona","tts":{"active":"openai-compatible","providers":[{"name":"openai-compatible","model":"gpt-4o-mini-tts","voice":"nova","instructions":"old instructions"}]},"log_level":"info","personality":"bmo","reduced_motion":false}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.TTS.Current().Model != "gpt-4o-mini-tts" || cfg.TTS.Current().Voice != "nova" {
		t.Fatalf("legacy config fields lost: %#v", cfg.TTS)
	}
}

func TestDefaultSetupComplete(t *testing.T) {
	cfg := Default()
	if !cfg.SetupComplete {
		t.Fatal("expected SetupComplete=true in default config")
	}
}

func TestDefaultDeviceContextAllEnabled(t *testing.T) {
	cfg := Default()
	dc := cfg.DeviceContext
	if !dc.Library || !dc.Saves || !dc.PlayLog || !dc.System || !dc.Achievements {
		t.Fatalf("expected all device context categories enabled by default, got %+v", dc)
	}
	if cfg.ProactiveTalk != ProactiveOff {
		t.Fatalf("expected proactive talk off by default, got %q", cfg.ProactiveTalk)
	}
}

func TestLoadConfigWithoutDeviceContextDefaultsEnabled(t *testing.T) {
	// Configs written before this feature have no device_context key; the
	// Load-over-Default merge must leave every category enabled.
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"mode":"idle","log_level":"info","personality":"bmo","stt":{},"chat":{},"tts":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	dc := cfg.DeviceContext
	if !dc.Library || !dc.Saves || !dc.PlayLog || !dc.System || !dc.Achievements {
		t.Fatalf("expected legacy config to default all categories on, got %+v", dc)
	}
}

func TestNormalizeProactiveTalk(t *testing.T) {
	cfg := Config{ProactiveTalk: "  CHATTY "}
	cfg.Normalize()
	if cfg.ProactiveTalk != ProactiveChatty {
		t.Fatalf("expected normalized chatty, got %q", cfg.ProactiveTalk)
	}
	cfg = Config{}
	cfg.Normalize()
	if cfg.ProactiveTalk != ProactiveOff {
		t.Fatalf("expected empty level normalized to off, got %q", cfg.ProactiveTalk)
	}
}

func TestValidateRejectsUnknownProactiveTalk(t *testing.T) {
	cfg := Default()
	cfg.ProactiveTalk = "constantly"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for unknown proactive talk level")
	}
}

func TestProactiveInterval(t *testing.T) {
	cases := map[string]time.Duration{
		ProactiveOff:        0,
		ProactiveChatty:     3 * time.Minute,
		ProactiveRegular:    30 * time.Minute,
		ProactiveOccasional: time.Hour,
		ProactiveRare:       3 * time.Hour,
		"bogus":             0,
		"":                  0,
	}
	for level, want := range cases {
		if got := ProactiveInterval(level); got != want {
			t.Errorf("ProactiveInterval(%q) = %v, want %v", level, got, want)
		}
	}
}

func TestRequestTimeoutDefaultsTo15(t *testing.T) {
	cfg := Default()
	cfg.Normalize()
	if cfg.RequestTimeout != 15 {
		t.Fatalf("want 15, got %d", cfg.RequestTimeout)
	}
}

func TestRequestTimeoutClampedWhenTooLow(t *testing.T) {
	cfg := Config{RequestTimeout: 5}
	cfg.Normalize()
	if cfg.RequestTimeout != 15 {
		t.Fatalf("want 15 (clamped), got %d", cfg.RequestTimeout)
	}
}

func TestRequestTimeoutClampedWhenTooHigh(t *testing.T) {
	cfg := Config{RequestTimeout: 999}
	cfg.Normalize()
	if cfg.RequestTimeout != 60 {
		t.Fatalf("want 60 (clamped), got %d", cfg.RequestTimeout)
	}
}

func TestSupportedRequestTimeoutsContains15And60(t *testing.T) {
	vals := SupportedRequestTimeouts()
	has15, has60 := false, false
	for _, v := range vals {
		if v == 15 {
			has15 = true
		}
		if v == 60 {
			has60 = true
		}
	}
	if !has15 {
		t.Fatal("SupportedRequestTimeouts missing 15")
	}
	if !has60 {
		t.Fatal("SupportedRequestTimeouts missing 60")
	}
}

func TestActiveModRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := Default()
	cfg.ActiveMod = "evil"
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.ActiveMod != "evil" {
		t.Fatalf("ActiveMod = %q, want evil", got.ActiveMod)
	}
}

func TestActiveModNormalizesWhitespace(t *testing.T) {
	cfg := Default()
	cfg.ActiveMod = "  evil  "
	cfg.Normalize()
	if cfg.ActiveMod != "evil" {
		t.Fatalf("ActiveMod = %q, want trimmed 'evil'", cfg.ActiveMod)
	}
}

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
	set.Active = "nope"
	if got := set.Current().Name; got != "openai" {
		t.Fatalf("Current().Name = %q, want openai (first fallback)", got)
	}
	set.Active = ""
	if got := set.Current().Name; got != "openai" {
		t.Fatalf("Current().Name = %q, want openai (empty fallback)", got)
	}
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
	empty.Cycle(1)
	if empty.Active != "" {
		t.Fatalf("empty Cycle set Active to %q", empty.Active)
	}
}

func TestProviderSetCycleFromUnresolvedActive(t *testing.T) {
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

func TestProviderSetJSONRoundTripAndValidate(t *testing.T) {
	cfg := Default()
	cfg.Mode = ModeAI
	cfg.STT = ProviderSet{Providers: []Provider{{Name: "a", Model: "whisper-1"}, {Name: "b", Model: "whisper-large"}}}
	cfg.Chat = ProviderSet{Active: "b", Providers: []Provider{{Name: "a", Model: "gpt-4o-mini"}, {Name: "b", Model: "gpt-4o"}}}
	cfg.TTS = ProviderSet{Providers: []Provider{{Name: "a", Model: "tts-1", Voice: "alloy"}}}

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

func TestContinuedConversationNormalizeDefaults(t *testing.T) {
	c := Config{ContinuedConversation: "BOGUS"}
	c.Normalize()
	if c.ContinuedConversation != ContinuedConvoOff {
		t.Fatalf("got %q want %q", c.ContinuedConversation, ContinuedConvoOff)
	}
}

func TestContinuedConversationNormalizeKeepsValid(t *testing.T) {
	c := Config{ContinuedConversation: ContinuedConvoLong}
	c.Normalize()
	if c.ContinuedConversation != ContinuedConvoLong {
		t.Fatalf("normalize clobbered valid value: %q", c.ContinuedConversation)
	}
}

func TestWakeWordValidatesTrigger(t *testing.T) {
	c := Default()
	c.Mode = ModeAI
	c.WakeWordEnabled = true
	c.InputTrigger = InputTriggerWakeWord
	c.STT = ProviderSet{Active: "s", Providers: []Provider{{Name: "s", Model: "whisper-1"}}}
	c.Chat = ProviderSet{Active: "c", Providers: []Provider{{Name: "c", Model: "gpt-4o-mini"}}}
	c.TTS = ProviderSet{Active: "t", Providers: []Provider{{Name: "t", Model: "tts-1"}}}
	if err := c.Validate(); err != nil {
		t.Fatalf("valid wake-word config rejected: %v", err)
	}
}
