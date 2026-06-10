package ui

import (
	"fmt"
	"strings"

	"github.com/carroarmato0/nextui-bmo/internal/config"
	"github.com/carroarmato0/nextui-bmo/internal/input"
)

type PTTButtonChoice struct {
	Code     string
	Label    string
	Selected bool
}

func buildPTTButtonChoices(selected []string) []PTTButtonChoice {
	selectedSet := make(map[string]struct{}, len(selected))
	for _, button := range selected {
		selectedSet[input.NormalizeButtonName(button)] = struct{}{}
	}

	buttons := config.SupportedPTTButtons()
	choices := make([]PTTButtonChoice, 0, len(buttons))
	for _, button := range buttons {
		code := input.NormalizeButtonName(button)
		choices = append(choices, PTTButtonChoice{
			Code:     code,
			Label:    input.ButtonLabel(code),
			Selected: func() bool { _, ok := selectedSet[code]; return ok }(),
		})
	}
	return choices
}

func setPTTButtonState(current []string, code string, selected bool) ([]string, error) {
	code = input.NormalizeButtonName(code)
	if _, ok := input.ParseButtonCode(code); !ok {
		return nil, fmt.Errorf("unknown ptt button %q", code)
	}
	seen := make(map[string]struct{}, len(current))
	buttons := make([]string, 0, len(current)+1)
	for _, button := range current {
		button = input.NormalizeButtonName(button)
		if button == "" || button == code {
			continue
		}
		if _, ok := input.ParseButtonCode(button); !ok {
			continue
		}
		if _, ok := seen[button]; ok {
			continue
		}
		seen[button] = struct{}{}
		buttons = append(buttons, button)
	}
	if selected {
		buttons = append(buttons, code)
	}
	if len(buttons) == 0 {
		return nil, fmt.Errorf("at least one ptt button must remain selected")
	}
	return config.NormalizePTTButtons(buttons), nil
}

func togglePTTButtonState(current []string, code string) ([]string, error) {
	code = input.NormalizeButtonName(code)
	if _, ok := input.ParseButtonCode(code); !ok {
		return nil, fmt.Errorf("unknown ptt button %q", code)
	}
	selected := make([]string, 0, len(current))
	found := false
	for _, button := range current {
		button = input.NormalizeButtonName(button)
		if button == "" {
			continue
		}
		if button == code {
			found = true
			continue
		}
		selected = append(selected, button)
	}
	if !found {
		selected = append(selected, code)
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("at least one ptt button must remain selected")
	}
	return config.NormalizePTTButtons(selected), nil
}

func joinPTTButtonLabels(buttons []string) string {
	if len(buttons) == 0 {
		return strings.Join(config.DefaultPTTButtons(), ", ")
	}
	labels := make([]string, 0, len(buttons))
	for _, button := range buttons {
		labels = append(labels, input.ButtonLabel(button))
	}
	return strings.Join(labels, ", ")
}
