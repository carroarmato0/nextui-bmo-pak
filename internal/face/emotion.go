package face

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FunctionalNames are the state-driven faces the assistant never requests as an
// emotion. They remain overridable as art, but are excluded from the emotion
// vocabulary advertised to the chat model.
var FunctionalNames = []string{ExprBlink, ExprListening, ExprThinking, ExprSpeaking, ExprSleeping, ExprLookAround, ExprWhistle}

func isFunctional(name string) bool {
	for _, f := range FunctionalNames {
		if f == name {
			return true
		}
	}
	return false
}

// EmotionNames returns the built-in emotion faces: every canonical name that is
// not a functional, state-driven face. This is the default vocabulary for the
// embedded BMO and any mod that inherits embedded faces.
func EmotionNames() []string {
	out := make([]string, 0, len(CanonicalNames))
	for _, n := range CanonicalNames {
		if !isFunctional(n) {
			out = append(out, n)
		}
	}
	return out
}

// EmotionFaceNamesInDir lists the emotion faces a mod ships on disk: the base
// names of *.svg files in dir, excluding functional faces and unsafe names,
// sorted. A missing or unreadable dir yields nil.
func EmotionFaceNamesInDir(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.EqualFold(filepath.Ext(e.Name()), ".svg") {
			continue
		}
		base := strings.ToLower(strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())))
		if !fileNameRe.MatchString(base) || isFunctional(base) {
			continue
		}
		out = append(out, base)
	}
	sort.Strings(out)
	return out
}
