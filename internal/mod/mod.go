package mod

import (
	"os"
	"path/filepath"
	"strings"
)

// DefaultID is the reserved folder name with overlay semantics.
const DefaultID = "default"

// Mod is a resolved mod: a folder under mods/ plus its (optional) manifest.
type Mod struct {
	ID        string   // folder name; the default entry uses DefaultID
	Root      string   // absolute path to mods/<id> (may not exist on disk)
	Manifest  Manifest // zero value when no mod.json
	IsDefault bool     // ID == DefaultID — overlay semantics
}

// DisplayName is what the Settings menu shows: the manifest name if set, else
// a friendly label for the default entry, else the folder id.
func (m Mod) DisplayName() string {
	if name := strings.TrimSpace(m.Manifest.Name); name != "" {
		return name
	}
	if m.IsDefault {
		return "BMO (Default)"
	}
	return m.ID
}

func (m Mod) PersonaPath() string { return filepath.Join(m.Root, "persona.txt") }
func (m Mod) VoicePath() string   { return filepath.Join(m.Root, "voice.txt") }
func (m Mod) QuotesPath() string  { return filepath.Join(m.Root, "quotes.txt") }
func (m Mod) FacesDir() string    { return filepath.Join(m.Root, "faces") }
func (m Mod) AudioDir() string    { return m.Root } // clips.Library appends "audio/"

// FacesHasSVG reports whether the mod's faces/ directory holds at least one
// .svg file. A missing or unreadable directory counts as none.
func (m Mod) FacesHasSVG() bool {
	entries, err := os.ReadDir(m.FacesDir())
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".svg") {
			return true
		}
	}
	return false
}

// SelfContained reports whether the mod owns its entire face set with no
// embedded fallback. True only for a named mod that ships ≥1 face; the default
// overlay and a named mod with no faces both inherit embedded BMO art.
func (m Mod) SelfContained() bool {
	return !m.IsDefault && m.FacesHasSVG()
}
