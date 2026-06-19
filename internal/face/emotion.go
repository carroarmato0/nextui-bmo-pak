package face

import (
	"io/fs"
	"sort"
	"strings"
)

// FunctionalNames are the state- and idle-driven faces the assistant never
// requests as an emotion: machine states (blink, listening, thinking, speaking,
// sleeping) plus idle-scheduler animations (look_around, whistle). They remain
// overridable as art, but are excluded from the emotion vocabulary advertised to
// the chat model.
var FunctionalNames = []string{ExprBlink, ExprListening, ExprThinking, ExprSpeaking, ExprSleeping, ExprLookAround, ExprWhistle}

func isFunctional(name string) bool {
	for _, f := range FunctionalNames {
		if f == name {
			return true
		}
	}
	return false
}

// EmotionNames returns the built-in emotion faces the chat model may request:
// every canonical name that is neither a functional, state-driven face nor a
// static (non-lip-syncing) one. Faces without an amplitude-driven mouth (e.g.
// crying, dead, dizzy, glitch, grimace, kiss, shout, teary) would sit frozen
// while BMO talks, so they are kept out of the vocabulary; they remain valid
// idle poses, just never something the model picks to speak behind. This is the
// default vocabulary for the embedded BMO and any mod that inherits embedded
// faces.
func EmotionNames() []string {
	anims := DefaultAnimations()
	out := make([]string, 0, len(CanonicalNames))
	for _, n := range CanonicalNames {
		if isFunctional(n) {
			continue
		}
		// Only lip-syncing faces (mouth driven by voice amplitude) can talk;
		// static faces have no animation def, idle faces are time-driven.
		if def, ok := anims[n]; !ok || def.Driver.Kind != DriverAmplitude {
			continue
		}
		out = append(out, n)
	}
	return out
}

// FaceNamesInFS lists every face a mod ships: base names of *.svg files at the
// root of fsys (rooted at the mod's faces/), sorted, INCLUDING functional/idle
// faces.
func FaceNamesInFS(fsys fs.FS) []string {
	return faceNamesInFS(fsys, false)
}

// EmotionFaceNamesInFS lists only the emotion faces (functional/idle excluded).
func EmotionFaceNamesInFS(fsys fs.FS) []string {
	return faceNamesInFS(fsys, true)
}

func faceNamesInFS(fsys fs.FS, excludeFunctional bool) []string {
	if fsys == nil {
		return nil
	}
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		dot := strings.LastIndex(name, ".")
		if dot < 0 || !strings.EqualFold(name[dot:], ".svg") {
			continue
		}
		base := strings.ToLower(name[:dot])
		if !fileNameRe.MatchString(base) {
			continue
		}
		if excludeFunctional && isFunctional(base) {
			continue
		}
		out = append(out, base)
	}
	sort.Strings(out)
	return out
}
