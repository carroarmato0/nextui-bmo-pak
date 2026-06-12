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
type Library struct {
	dir  string
	logf func(string, ...any)
}

// NewLibrary returns a Library that checks dir for overrides before falling
// back to embedded defaults. dir may not exist; missing dirs are silently
// ignored.
func NewLibrary(dir string) *Library {
	return &Library{dir: dir, logf: func(string, ...any) {}}
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
				continue // blank file → fall through to embedded
			}
			return data, true
		}
	}

	data, ok := defaultBytes(canonical)
	if !ok {
		return nil, false
	}
	return data, false
}
