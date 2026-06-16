package face

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Library resolves expression names to SVG bytes. The embedded defaults are
// the source of truth; an optional on-disk faces/ directory overrides per file.
// In self-contained mode the embedded defaults are NOT used: a named mod owns
// its whole face set, and a missing expression folds to the mod's own neutral.
type Library struct {
	dir           string
	selfContained bool
	logf          func(string, ...any)
}

// NewLibrary returns a Library that checks dir for overrides before falling
// back to embedded defaults. dir may not exist; missing dirs are silently
// ignored.
func NewLibrary(dir string) *Library {
	return NewLibraryMode(dir, false)
}

// NewLibraryMode is like NewLibrary but, when selfContained is true, disables
// the embedded fallback: only the on-disk faces/ directory is consulted, and a
// missing expression folds to the directory's neutral.svg (or, if that is also
// missing, resolves to nothing so the renderer draws its plain fallback).
func NewLibraryMode(dir string, selfContained bool) *Library {
	return &Library{dir: dir, selfContained: selfContained, logf: func(string, ...any) {}}
}

// SetLogf sets the logger used for warnings (e.g. parse failures of overrides).
func (l *Library) SetLogf(f func(string, ...any)) {
	if f != nil {
		l.logf = f
	}
}

// fileNameRe accepts only safe filenames (no path separators or dots).
var fileNameRe = regexp.MustCompile(`^[a-z0-9_-]+$`)

// Bytes returns SVG bytes for the given expression. It checks:
//
//	faces/<name>.svg → faces/<canonical>.svg → embedded default
//
// Returns (data, true) when a non-blank disk file was found, (data, false)
// when using the embedded default.
func (l *Library) Bytes(expr string) ([]byte, bool) {
	raw := strings.ToLower(strings.TrimSpace(expr))
	canonical := Canonical(raw)

	// Build candidate names: raw first (if distinct), then canonical.
	names := []string{canonical}
	if canonical != raw && fileNameRe.MatchString(raw) {
		names = []string{raw, canonical}
	}

	if l.dir != "" {
		for _, name := range names {
			if !fileNameRe.MatchString(name) {
				continue
			}
			data, err := os.ReadFile(filepath.Join(l.dir, name+".svg"))
			if err != nil {
				continue
			}
			if len(bytes.TrimSpace(data)) == 0 {
				l.logf("face: override %s.svg is blank; using default", name)
				continue
			}
			return data, true
		}
	}

	if l.selfContained {
		// Owns its whole set: no embedded fallback. Fold a missing expression
		// to the mod's own neutral, if it ships one.
		if canonical != ExprNeutral && l.dir != "" {
			data, err := os.ReadFile(filepath.Join(l.dir, ExprNeutral+".svg"))
			if err == nil && len(bytes.TrimSpace(data)) > 0 {
				return data, true
			}
		}
		return nil, false
	}

	data, ok := defaultBytes(canonical)
	if !ok {
		return nil, false
	}
	return data, false
}

// Resolve returns the cache key / load name for expr: the raw (sanitized) name
// when a matching on-disk face exists, so a self-contained mod's custom-named
// SVG renders under its own name; otherwise the canonical name.
func (l *Library) Resolve(expr string) string {
	raw := strings.ToLower(strings.TrimSpace(expr))
	if l.dir != "" && fileNameRe.MatchString(raw) {
		if _, err := os.Stat(filepath.Join(l.dir, raw+".svg")); err == nil {
			return raw
		}
	}
	return Canonical(raw)
}

// rawBytes returns SVG bytes for a literal (non-canonicalized) name: a
// faces/<name>.svg override if present, else the embedded assets/<name>.svg.
// Used by the animation engine, where frame and template names are literal
// asset names rather than expression aliases. Self-contained libraries do not
// fall back to embedded assets.
func (l *Library) rawBytes(name string) ([]byte, bool) {
	if !fileNameRe.MatchString(name) {
		return nil, false
	}
	if l.dir != "" {
		if data, err := os.ReadFile(filepath.Join(l.dir, name+".svg")); err == nil && len(bytes.TrimSpace(data)) > 0 {
			return data, true
		}
	}
	if l.selfContained {
		return nil, false
	}
	return defaultBytes(name)
}
