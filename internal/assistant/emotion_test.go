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

func TestParseEmotion(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		wantClean string
		wantEmo   Expression
	}{
		{"no directive", "Hello there!", "Hello there!", ""},
		{"leading directive", "[happy] Hello there!", "Hello there!", ExpressionHappy},
		{"leading no space", "[happy]Hello", "Hello", ExpressionHappy},
		{"embedded directive", "Oh [excited] I love it", "Oh I love it", ExpressionExcited},
		{"trailing directive", "Goodbye [sad]", "Goodbye", ExpressionSad},
		{"case insensitive", "[HAPPY] hi", "hi", ExpressionHappy},
		{"unknown bracket kept", "Wait [pauses] then go", "Wait [pauses] then go", ""},
		{"numeric bracket kept", "See note [1] here", "See note [1] here", ""},
		{"multiple first wins all stripped", "[sad] no [happy] yes", "no yes", ExpressionSad},
		{"only a directive", "[happy]", "", ExpressionHappy},
		{"directive with surrounding spaces tidy", "hi  [happy]  there", "hi there", ExpressionHappy},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clean, emo := ParseEmotion(tt.in)
			if clean != tt.wantClean {
				t.Errorf("clean = %q, want %q", clean, tt.wantClean)
			}
			if emo != tt.wantEmo {
				t.Errorf("emotion = %q, want %q", emo, tt.wantEmo)
			}
		})
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
