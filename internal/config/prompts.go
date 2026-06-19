package config

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
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

// LoadPromptFS returns the trimmed content of name within fsys if present and
// non-blank, else the built-in def. Mirrors LoadPromptFile for fs.FS sources.
func LoadPromptFS(fsys fs.FS, name, def string) string {
	if fsys != nil {
		if data, err := fs.ReadFile(fsys, name); err == nil {
			if content := strings.TrimSpace(string(data)); content != "" {
				return content
			}
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

// CheckOverrides validates a mod's override files in fsys, returning any
// problems: persona.txt / voice.txt / quotes.txt present-but-blank, and any
// faces/*.svg that is not valid XML. A missing file is not an error.
func CheckOverrides(fsys fs.FS) []error {
	var errs []error

	checkText := func(name, label string) {
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return // absent is fine
		}
		if strings.TrimSpace(string(data)) == "" {
			errs = append(errs, fmt.Errorf("%s exists but is blank", label))
		}
	}

	checkText("persona.txt", "persona.txt")
	checkText("voice.txt", "voice.txt")
	checkText("quotes.txt", "quotes.txt")

	entries, err := fs.ReadDir(fsys, "faces")
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, fmt.Errorf("faces dir: %w", err))
		}
		return errs
	}

	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".svg") {
			continue
		}
		data, err := fs.ReadFile(fsys, "faces/"+e.Name())
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
