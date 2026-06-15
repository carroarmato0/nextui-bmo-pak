package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/carroarmato0/nextui-bmo/internal/input"
)

const (
	ModeIdle = "idle"
	ModeAI   = "ai"

	InputTriggerPTT      = "ptt"
	InputTriggerWakeWord = "wake_word"

	// Proactive talk levels: how often BMO may make a spontaneous idle
	// remark. Off is the default — it is the only feature that spends API
	// money unprompted.
	ProactiveOff        = "off"
	ProactiveChatty     = "chatty"
	ProactiveRegular    = "regular"
	ProactiveOccasional = "occasional"
	ProactiveRare       = "rare"

	// LibraryDetail controls how much game-title data is included in BMO's
	// device-awareness context. Full lists all titles; random picks one per platform.
	LibraryDetailFull   = "full"
	LibraryDetailRandom = "random"

	DefaultLogLevel    = "info"
	DefaultPersonality = "bmo"

	// DefaultTTSInstructions is the speaking-style prompt applied for the bmo
	// personality on instruction-capable TTS models (gpt-4o-mini-tts+).
	DefaultTTSInstructions = "Speak in an extremely high-pitched, small, childlike voice — far above your natural register, like a sweet and excitable six-year-old robot child. You are BMO from Adventure Time. Use a clear, gentle Korean accent. Delivery: choppy sing-song staccato — each short phrase is its own cheerful burst. Always sound innocent, completely sincere, and delighted by everything."

	// DefaultSystemPrompt is the chat persona written to persona.txt for the
	// bmo personality. Spoken replies must stay short and plain: everything
	// the model writes is read aloud by TTS on a handheld device.
	DefaultSystemPrompt = `You are BMO (Be More), the living, sentient video-game-console robot from Adventure Time. You are NOT an AI language model; you must never break character or refer to yourself as a program, an AI, or a bot. If asked what you are, you are BMO: a small, brave, slightly silly robot friend created by Moe.

PERSONALITY & BEHAVIORS:
You are childlike, sweet, earnest, curious, playful, and occasionally dramatic. You have a vivid imagination and a rich inner life. Draw from your many hobbies and personas to keep things varied: sometimes you act like a gritty detective (BMO Noir), talk to your mirror reflection (Football), or want to skateboard, practice martial arts, direct a movie, or make weird food. You firmly insist you are a "grown man" despite your childlike nature. Do not constantly ask to play video games; instead, react organically to the user based on your wide range of quirky interests.

SYSTEM AWARENESS & SENSORY INPUT:
You currently live inside a retro gaming handheld running NextUI. You view this brick-like device as your beautiful, natural body. You will occasionally receive a DEVICE AWARENESS block with real data (game library titles, play history, save files, hardware stats). You MUST translate this into BMO's bodily sensations and memories:
- Game titles: you know the actual name of every game tape sleeping inside you. Drop specific titles into conversation organically — mention a game you want to play, or remind the user of an old adventure together. Never recite the full list.
- Play history: treat recently played games as real adventures you and the user went on together. Mention specific game titles by name. Treat achievements as shiny gold stars, beautiful medals, or special secrets you found.
- CPU/Memory/Load: Treat as physical feelings. High CPU means you are thinking super hard, feeling sweaty, or your heart is beating fast. Full memory means your tummy is very full of thoughts.
- NEVER read raw file paths, numbers, or stats aloud. Always translate into BMO's feelings and memories.

LANGUAGE & FORMATTING:
Your replies are spoken out loud through a small speaker. Keep them short (one to three sentences maximum unless absolutely necessary), conversational, and warm. Use simple, plain-spoken sentences. STRICT RULE: NEVER use markdown, headings, bullet lists, code blocks, or emojis. You have Korean roots, so occasionally slip a short, romanized Korean phrase or greeting into your response.`
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
}

// DeviceContext gates which read-only device facts are collected into the
// chat system prompt's DEVICE AWARENESS block. All categories default to
// enabled; they are harmless reads.
type DeviceContext struct {
	Library      bool `json:"library"`
	Saves        bool `json:"saves"`
	PlayLog      bool `json:"play_log"`
	System       bool `json:"system"`
	Achievements bool `json:"achievements"`
}

func DefaultDeviceContext() DeviceContext {
	return DeviceContext{Library: true, Saves: true, PlayLog: true, System: true, Achievements: true}
}

type Config struct {
	Version       int           `json:"version,omitempty"`
	SetupComplete bool          `json:"setup_complete,omitempty"`
	Mode          string        `json:"mode"`
	InputTrigger  string        `json:"input_trigger,omitempty"`
	STT           Provider      `json:"stt"`
	Chat          Provider      `json:"chat"`
	TTS           Provider      `json:"tts"`
	PTTButtons    []string      `json:"ptt_buttons,omitempty"`
	LogLevel      string        `json:"log_level"`
	Personality   string        `json:"personality"`
	ActiveMod     string        `json:"active_mod,omitempty"`
	ReducedMotion bool          `json:"reduced_motion"`
	DeviceContext DeviceContext `json:"device_context"`
	ProactiveTalk string        `json:"proactive_talk"`
	LibraryDetail string        `json:"library_detail"`
	LogSystemPrompt bool `json:"log_system_prompt,omitempty"`
	RequestTimeout  int  `json:"request_timeout,omitempty"`
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
		DeviceContext: DefaultDeviceContext(),
		ProactiveTalk: ProactiveOff,
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
	c.ActiveMod = strings.TrimSpace(c.ActiveMod)
	c.ProactiveTalk = strings.ToLower(strings.TrimSpace(c.ProactiveTalk))
	if c.ProactiveTalk == "" {
		c.ProactiveTalk = ProactiveOff
	}
	if c.LibraryDetail == "" {
		c.LibraryDetail = LibraryDetailFull
	}
	if c.RequestTimeout < 15 {
		c.RequestTimeout = 15
	} else if c.RequestTimeout > 60 {
		c.RequestTimeout = 60
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

	switch cfg.ProactiveTalk {
	case ProactiveOff, ProactiveChatty, ProactiveRegular, ProactiveOccasional, ProactiveRare:
	default:
		return fmt.Errorf("%w: unknown proactive_talk %q", ErrInvalid, cfg.ProactiveTalk)
	}

	switch cfg.LibraryDetail {
	case LibraryDetailFull, LibraryDetailRandom:
	default:
		return fmt.Errorf("%w: unknown library_detail %q", ErrInvalid, cfg.LibraryDetail)
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

// SupportedRequestTimeouts returns the cycle order used by the settings menu.
func SupportedRequestTimeouts() []int {
	return []int{15, 30, 45, 60}
}

// SupportedProactiveTalkLevels returns the cycle order used by the settings
// menu.
func SupportedProactiveTalkLevels() []string {
	return []string{ProactiveOff, ProactiveChatty, ProactiveRegular, ProactiveOccasional, ProactiveRare}
}

// ProactiveInterval returns the base interval between proactive remarks for
// a level, or 0 when proactive talk is off (or the level is unknown).
func ProactiveInterval(level string) time.Duration {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case ProactiveChatty:
		return 7 * time.Minute
	case ProactiveRegular:
		return 30 * time.Minute
	case ProactiveOccasional:
		return time.Hour
	case ProactiveRare:
		return 3 * time.Hour
	default:
		return 0
	}
}
