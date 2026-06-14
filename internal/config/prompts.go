package config

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
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

// CheckOverrides validates override files in homeDir and returns any errors
// found. It checks persona.txt, voice.txt, and quotes.txt for non-blank
// content, and validates all *.svg files in the faces/ subdirectory as
// well-formed XML.
func CheckOverrides(homeDir string) []error {
	var errs []error

	checkText := func(path, label string) {
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			return
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", label, err))
			return
		}
		if strings.TrimSpace(string(data)) == "" {
			errs = append(errs, fmt.Errorf("%s exists but is blank", label))
		}
	}

	checkText(PersonaPath(homeDir), "persona.txt")
	checkText(VoicePath(homeDir), "voice.txt")
	checkText(QuotesPath(homeDir), "quotes.txt")

	entries, err := os.ReadDir(FacesDir(homeDir))
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, fmt.Errorf("faces dir: %w", err))
		}
		return errs
	}

	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".svg" {
			continue
		}
		p := filepath.Join(FacesDir(homeDir), e.Name())
		data, err := os.ReadFile(p)
		if err != nil {
			errs = append(errs, fmt.Errorf("faces/%s: %w", e.Name(), err))
			continue
		}
		if !isValidXML(data) {
			errs = append(errs, fmt.Errorf("faces/%s: not valid XML", e.Name()))
		}
	}
	return errs
}

func isValidXML(data []byte) bool {
	d := xml.NewDecoder(bytes.NewReader(data))
	hasElement := false
	for {
		tok, err := d.Token()
		if err == io.EOF {
			return hasElement
		}
		if err != nil {
			return false
		}
		if _, ok := tok.(xml.StartElement); ok {
			hasElement = true
		}
	}
}
