package ui

import (
	"strings"

	"github.com/carroarmato0/nextui-bmo/internal/config"
)

const (
	providerKindSTT  = "stt"
	providerKindChat = "chat"
	providerKindTTS  = "tts"
)

// providerSummaryLabel renders a one-line, read-only summary of a provider for
// the Settings menu. Providers (model, endpoint, key) are configured in
// config.json, not in the UI.
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
