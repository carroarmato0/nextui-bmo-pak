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
