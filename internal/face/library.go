package face

import (
	"bytes"
	"io/fs"
	"regexp"
	"strings"
)

// Library resolves expression names to SVG bytes. The embedded defaults are
// the source of truth; an optional fs.FS (rooted at the mod's faces/) overrides
// per file. In self-contained mode the embedded defaults are NOT used: a named
// mod owns its whole face set, and a missing expression folds to the mod's own
// neutral.
type Library struct {
	fsys          fs.FS // rooted at the mod's faces/; nil = embedded only
	selfContained bool
	logf          func(string, ...any)
}

// NewLibrary builds an overlay library: on-disk faces override embedded
// defaults per file. fsys may be nil (embedded only).
func NewLibrary(fsys fs.FS) *Library {
	return NewLibraryMode(fsys, false)
}

// NewLibraryMode builds a library. When selfContained is true, embedded
// fallback is disabled: only fsys is consulted and a missing expression folds
// to neutral.svg.
func NewLibraryMode(fsys fs.FS, selfContained bool) *Library {
	return &Library{fsys: fsys, selfContained: selfContained, logf: func(string, ...any) {}}
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

	if l.fsys != nil {
		for _, name := range names {
			if !fileNameRe.MatchString(name) {
				continue
			}
			data, err := fs.ReadFile(l.fsys, name+".svg")
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
		if canonical != ExprNeutral && l.fsys != nil {
			data, err := fs.ReadFile(l.fsys, ExprNeutral+".svg")
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
	if l.fsys != nil && fileNameRe.MatchString(raw) {
		if _, err := fs.Stat(l.fsys, raw+".svg"); err == nil {
			return raw
		}
	}
	return Canonical(raw)
}

// Source reports where expr's rendered bytes come from: "mod-override" when an
// on-disk faces/<name>.svg (or, for a self-contained mod, its own neutral fold)
// supplies them, "embedded-default" when the built-in asset is used, or "none"
// when nothing resolves (self-contained mod with no matching face). It performs
// the same lookup as Bytes and does no logging.
// Source origin strings returned by Source.
const (
	SourceModOverride     = "mod-override"
	SourceEmbeddedDefault = "embedded-default"
	SourceNone            = "none"
)

func (l *Library) Source(expr string) string {
	data, fromDisk := l.Bytes(expr)
	switch {
	case fromDisk:
		return SourceModOverride
	case data != nil:
		return SourceEmbeddedDefault
	default:
		return SourceNone
	}
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
	if l.fsys != nil {
		if data, err := fs.ReadFile(l.fsys, name+".svg"); err == nil && len(bytes.TrimSpace(data)) > 0 {
			return data, true
		}
	}
	if l.selfContained {
		return nil, false
	}
	return defaultBytes(name)
}
