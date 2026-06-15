package assistant

import (
	"testing"

	"github.com/carroarmato0/nextui-bmo/internal/face"
)

// Every advertised emotion must resolve to its OWN face. If face.Canonical
// folds it onto something else (or the asset is missing) the model would be
// told about a face BMO cannot actually show.
func TestEmotionVocabularyResolvesToItself(t *testing.T) {
	if len(EmotionVocabulary) == 0 {
		t.Fatal("EmotionVocabulary is empty")
	}
	seen := map[Expression]bool{}
	for _, e := range EmotionVocabulary {
		if seen[e] {
			t.Errorf("duplicate vocabulary entry %q", e)
		}
		seen[e] = true
		if got := face.Canonical(string(e)); got != string(e) {
			t.Errorf("face.Canonical(%q) = %q, want %q (not a self-resolving face)", e, got, e)
		}
	}
}

// The functional, state-driven faces must NOT be advertised to the model.
func TestEmotionVocabularyExcludesFunctionalFaces(t *testing.T) {
	excluded := []Expression{
		ExpressionListening, ExpressionThinking, ExpressionSpeaking,
		ExpressionSleeping, ExpressionBlink, ExpressionLookAround, ExpressionWhistle,
	}
	for _, e := range EmotionVocabulary {
		for _, x := range excluded {
			if e == x {
				t.Errorf("vocabulary must not include functional face %q", e)
			}
		}
	}
}
