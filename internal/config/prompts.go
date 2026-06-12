package config

import (
	"errors"
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

// LoadPromptFile returns the trimmed content of path if it exists and is
// non-blank, otherwise returns the trimmed built-in default def.
func LoadPromptFile(path, def string) string {
	if data, err := os.ReadFile(path); err == nil {
		if content := strings.TrimSpace(string(data)); content != "" {
			return content
		}
	}
	return strings.TrimSpace(def)
}

// RemoveOverrides deletes override files so the app falls back to built-in
// defaults. Missing files are silently ignored; returns the first real error.
func RemoveOverrides(paths ...string) error {
	var firstErr error
	for _, p := range paths {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}
