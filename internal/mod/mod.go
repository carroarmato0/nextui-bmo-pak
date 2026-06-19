package mod

import (
	"archive/zip"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// DefaultID is the overlay mod that falls back per-asset to embedded BMO.
const DefaultID = "default"

// Mod is a selectable character under mods/: a directory or a .zip archive.
type Mod struct {
	ID        string
	Root      string // mods/<id> or mods/<id>.zip — identity, logging, dir-only tools
	Manifest  Manifest
	IsDefault bool

	// FS is rooted at the mod's contents (mod.json at root, faces/ and audio/
	// as subtrees). Populated by Open; nil until then.
	FS fs.FS

	closer func() error
}

// DisplayName is the manifest name if set, else a friendly default.
func (m Mod) DisplayName() string {
	if name := strings.TrimSpace(m.Manifest.Name); name != "" {
		return name
	}
	if m.IsDefault {
		return "BMO (Default)"
	}
	return m.ID
}

// Path accessors operate on Root and are valid only for directory mods.
// PersonaPath/VoicePath feed cmd/generate-audio (a directory-only build tool);
// PersonaPath/VoicePath/QuotesPath also feed config.RemoveOverrides (the
// settings "reset overrides" action). FacesDir/AudioDir are retained for
// tooling and tests — runtime reads go through the mod's fs.FS, not these.
func (m Mod) PersonaPath() string { return filepath.Join(m.Root, "persona.txt") }
func (m Mod) VoicePath() string   { return filepath.Join(m.Root, "voice.txt") }
func (m Mod) QuotesPath() string  { return filepath.Join(m.Root, "quotes.txt") }
func (m Mod) FacesDir() string    { return filepath.Join(m.Root, "faces") }
func (m Mod) AudioDir() string    { return filepath.Join(m.Root, "audio") }

// Open populates m.FS from Root. Directories use os.DirFS; .zip archives are
// opened and rooted at their top-level <id>/ folder when present, otherwise at
// the archive root (logging a warning via logf, which may be nil). Callers must
// Close the mod when done so zip file descriptors are released.
func (m *Mod) Open(logf func(format string, args ...any)) error {
	if strings.HasSuffix(m.Root, ".zip") {
		zr, err := zip.OpenReader(m.Root)
		if err != nil {
			return err
		}
		var fsys fs.FS = zr
		if info, err := fs.Stat(zr, m.ID); err == nil && info.IsDir() {
			if sub, err := fs.Sub(zr, m.ID); err == nil {
				fsys = sub
			}
		} else if logf != nil {
			logf("mod %q: zip has no top-level %q/ folder; reading from archive root", m.ID, m.ID)
		}
		m.FS = fsys
		m.closer = zr.Close
		return nil
	}
	m.FS = os.DirFS(m.Root)
	m.closer = nil
	return nil
}

// Close releases the mod's source (a no-op for directories).
func (m *Mod) Close() error {
	if m.closer != nil {
		err := m.closer()
		m.closer = nil
		return err
	}
	return nil
}

// FacesHasSVG reports whether the mod ships at least one faces/*.svg. Requires
// Open to have populated FS.
func (m Mod) FacesHasSVG() bool {
	if m.FS == nil {
		return false
	}
	entries, err := fs.ReadDir(m.FS, "faces")
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

// SelfContained reports whether the mod ships its own faces and is not the
// overlay default (so it must not inherit embedded faces/animations).
func (m Mod) SelfContained() bool {
	return !m.IsDefault && m.FacesHasSVG()
}
