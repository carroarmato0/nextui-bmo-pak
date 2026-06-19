// Package mod discovers and resolves BMO mods: subfolders of mods/ that
// customize persona, voice, quotes, faces, and audio. mods/default has
// overlay semantics (per-asset fallback to embedded BMO); any other folder
// is a self-contained character.
package mod

import (
	"encoding/json"
	"io/fs"
)

// CurrentAPIVersion is the mod-format contract this build implements. Bump it
// in a future spec when a compatibility-breaking change to the mod format
// ships; add version-specific handling keyed off Manifest.EffectiveAPIVersion.
const CurrentAPIVersion = 1

// Manifest is the optional mod.json. Every field is optional; a missing or
// malformed file yields the zero value and the mod still loads.
type Manifest struct {
	APIVersion  int    `json:"apiVersion"`  // mod-format version; 0/absent => 1
	Name        string `json:"name"`        // display name override
	Author      string `json:"author"`      // shown in Settings
	Description string `json:"description"` // shown in Settings
	Version     string `json:"version"`     // author's own free-form release string

	Emotions map[string]string `json:"emotions,omitempty"` // emotion name -> LLM description

	// Animations maps an expression name to its raw animation JSON. Parsing of
	// the inner shape is deferred to internal/face (ParseAnimations) so this
	// package stays free of rendering concerns and tolerant of unknown fields.
	Animations map[string]json.RawMessage `json:"animations,omitempty"`
}

// EffectiveAPIVersion returns the declared apiVersion, treating absent (0) as
// 1 — the version current when the field was introduced. This default is
// frozen forever so that undeclared mods keep their original semantics when a
// newer API ships.
func (m Manifest) EffectiveAPIVersion() int {
	if m.APIVersion <= 0 {
		return 1
	}
	return m.APIVersion
}

// LoadManifest reads and parses mod.json from the root of fsys. A missing or
// malformed file yields the zero Manifest (mods need no manifest).
func LoadManifest(fsys fs.FS) Manifest {
	data, err := fs.ReadFile(fsys, "mod.json")
	if err != nil {
		return Manifest{}
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}
	}
	return m
}
