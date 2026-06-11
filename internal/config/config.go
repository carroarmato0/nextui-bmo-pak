package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/carroarmato0/nextui-bmo/internal/input"
)

const (
	ModeIdle = "idle"
	ModeAI   = "ai"

	InputTriggerPTT      = "ptt"
	InputTriggerWakeWord = "wake_word"

	DefaultLogLevel    = "info"
	DefaultPersonality = "bmo"

	// DefaultTTSInstructions is the speaking-style prompt applied for the bmo
	// personality on instruction-capable TTS models (gpt-4o-mini-tts+).
	DefaultTTSInstructions = "Speak like BMO from Adventure Time: a cheerful, childlike little robot. Bright, playful, innocent and curious, with a light synthetic quality. Keep the energy high and friendly."
)

var defaultPTTButtons = []string{"BTN_EAST"} // physical A button on TrimUI

var supportedPTTButtons = []string{
	"BTN_SOUTH",
	"BTN_EAST",
	"BTN_C",
	"BTN_NORTH",
	"BTN_WEST",
	"BTN_TL",
	"BTN_TR",
	"BTN_TL2",
	"BTN_TR2",
	"BTN_SELECT",
	"BTN_START",
	"BTN_MODE",
	"BTN_THUMBL",
	"BTN_THUMBR",
}

var ErrNotFound = errors.New("config not found")
var ErrInvalid = errors.New("invalid config")

type Provider struct {
	Name    string `json:"name,omitempty"`
	Model   string `json:"model,omitempty"`
	Voice   string `json:"voice,omitempty"`
	BaseURL string `json:"base_url,omitempty"`
	APIKey  string `json:"api_key,omitempty"`
	// Instructions steer the speaking style on instruction-capable TTS
	// models (gpt-4o-mini-tts and newer). Only meaningful for the TTS provider.
	Instructions string `json:"instructions,omitempty"`
}

type Config struct {
	Version       int      `json:"version,omitempty"`
	SetupComplete bool     `json:"setup_complete,omitempty"`
	Mode          string   `json:"mode"`
	InputTrigger  string   `json:"input_trigger,omitempty"`
	STT           Provider `json:"stt"`
	Chat          Provider `json:"chat"`
	TTS           Provider `json:"tts"`
	PTTButtons    []string `json:"ptt_buttons,omitempty"`
	LogLevel      string   `json:"log_level"`
	Personality   string   `json:"personality"`
	ReducedMotion bool     `json:"reduced_motion"`
	SystemPrompt  string   `json:"system_prompt,omitempty"`
}

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

func DefaultPTTButtons() []string {
	return append([]string(nil), defaultPTTButtons...)
}

func SupportedPTTButtons() []string {
	return append([]string(nil), supportedPTTButtons...)
}

func Path(homeDir string) string {
	return filepath.Join(homeDir, "config.json")
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Default(), ErrNotFound
		}
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	cfg := Default()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	cfg.Normalize()
	return cfg, nil
}

func Save(path string, cfg Config) error {
	cfg.Normalize()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func (c *Config) Normalize() {
	if c.Version <= 0 {
		c.Version = 1
	}
	if c.Mode == "" {
		c.Mode = ModeIdle
	}
	if c.InputTrigger == "" {
		c.InputTrigger = InputTriggerPTT
	}
	if len(c.PTTButtons) == 0 {
		c.PTTButtons = DefaultPTTButtons()
	}
	if c.LogLevel == "" {
		c.LogLevel = DefaultLogLevel
	}
	if c.Personality == "" {
		c.Personality = DefaultPersonality
	}
	// The bmo personality ships with an opinionated speaking style; the
	// provider layer drops it automatically for models that reject it.
	if c.TTS.Instructions == "" && c.Personality == DefaultPersonality {
		c.TTS.Instructions = DefaultTTSInstructions
	}
}

func (c Config) Validate() error {
	cfg := c
	cfg.Normalize()

	if cfg.Mode != ModeIdle && cfg.Mode != ModeAI {
		return fmt.Errorf("%w: unknown mode %q", ErrInvalid, cfg.Mode)
	}
	if cfg.InputTrigger != InputTriggerPTT && cfg.InputTrigger != InputTriggerWakeWord {
		return fmt.Errorf("%w: unknown input trigger %q", ErrInvalid, cfg.InputTrigger)
	}
	if err := ValidatePTTButtons(cfg.PTTButtons); err != nil {
		return err
	}

	if cfg.Mode == ModeAI {
		if err := validateAIProvider("stt", cfg.STT); err != nil {
			return err
		}
		if err := validateAIProvider("chat", cfg.Chat); err != nil {
			return err
		}
		if err := validateAIProvider("tts", cfg.TTS); err != nil {
			return err
		}
	}

	return nil
}

func ValidatePTTButtons(buttons []string) error {
	for _, button := range buttons {
		if _, ok := input.ParseButtonCode(button); !ok {
			return fmt.Errorf("%w: unknown ptt button %q", ErrInvalid, button)
		}
	}
	return nil
}

func NormalizePTTButtons(buttons []string) []string {
	if len(buttons) == 0 {
		return DefaultPTTButtons()
	}
	seen := make(map[string]struct{}, len(buttons))
	out := make([]string, 0, len(buttons))
	for _, button := range buttons {
		button = strings.ToUpper(strings.TrimSpace(button))
		if button == "" {
			continue
		}
		if _, ok := seen[button]; ok {
			continue
		}
		seen[button] = struct{}{}
		out = append(out, button)
	}
	if len(out) == 0 {
		return DefaultPTTButtons()
	}
	return out
}

func validateAIProvider(kind string, p Provider) error {
	missing := make([]string, 0, 2)
	if strings.TrimSpace(p.Name) == "" {
		missing = append(missing, "name")
	}
	if strings.TrimSpace(p.Model) == "" {
		missing = append(missing, "model")
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %s provider missing %s", ErrInvalid, kind, strings.Join(missing, ", "))
}

func (c Config) Redacted() Config {
	cfg := c
	cfg.STT.APIKey = ""
	cfg.Chat.APIKey = ""
	cfg.TTS.APIKey = ""
	return cfg
}

func (c Config) Secrets() []string {
	secrets := make([]string, 0, 3)
	seen := map[string]struct{}{}
	for _, value := range []string{c.STT.APIKey, c.Chat.APIKey, c.TTS.APIKey} {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		secrets = append(secrets, value)
	}
	return secrets
}

func (c Config) UsesAI() bool {
	return strings.EqualFold(strings.TrimSpace(c.Mode), ModeAI)
}

func (p Provider) IsConfigured() bool {
	return strings.TrimSpace(p.Name) != "" && strings.TrimSpace(p.Model) != ""
}
