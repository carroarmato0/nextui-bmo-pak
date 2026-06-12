package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PersonaPath returns the location of the chat persona prompt file.
func PersonaPath(homeDir string) string {
	return filepath.Join(homeDir, "persona.txt")
}

// VoicePath returns the location of the TTS speaking-style prompt file.
func VoicePath(homeDir string) string {
	return filepath.Join(homeDir, "voice.txt")
}

// FacesDir returns the directory where mod SVG overrides are looked up.
// The directory may not exist; callers should tolerate that gracefully.
func FacesDir(homeDir string) string {
	return filepath.Join(homeDir, "faces")
}

// EnsurePromptFile guarantees a usable prompt file at path and returns its
// content. A missing file is created containing def; a blank file is filled
// with def; a file with content is returned as-is and never overwritten.
func EnsurePromptFile(path, def string) (string, error) {
	def = strings.TrimSpace(def)
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("read prompt file: %w", err)
	}
	if err == nil {
		if content := strings.TrimSpace(string(data)); content != "" {
			return content, nil
		}
	}
	if err := WritePromptFile(path, def); err != nil {
		return "", err
	}
	return def, nil
}

// WritePromptFile writes content to the prompt file at path, overwriting any
// existing content. Used by EnsurePromptFile and the settings menu's
// restore-defaults action.
func WritePromptFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create prompt dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o600); err != nil {
		return fmt.Errorf("write prompt file: %w", err)
	}
	return nil
}
