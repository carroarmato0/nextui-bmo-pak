package assistant

import (
	"strings"
	"testing"

	"github.com/carroarmato0/nextui-bmo/internal/face"
)

func builtinVocab() []EmotionEntry {
	return BuildEmotionVocabulary(face.EmotionNames(), nil, nil)
}

func TestBuildEmotionVocabularyOverlay(t *testing.T) {
	v := BuildEmotionVocabulary([]string{"happy", "sad"}, []string{"sad", "grumpy"}, map[string]string{"grumpy": "sulky"})
	var names []string
	for _, e := range v {
		names = append(names, e.Name)
	}
	want := []string{"happy", "sad", "grumpy"} // builtin first, then new disk names, deduped
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("names = %v, want %v", names, want)
	}
	if v[2].Description != "sulky" {
		t.Fatalf("grumpy description = %q, want sulky", v[2].Description)
	}
}

func TestBuildEmotionVocabularySelfContained(t *testing.T) {
	v := BuildEmotionVocabulary(nil, []string{"grumpy", "happy"}, nil)
	if len(v) != 2 || v[0].Name != "grumpy" || v[1].Name != "happy" {
		t.Fatalf("self-contained vocab = %+v, want [grumpy happy]", v)
	}
}

func TestParseEmotion(t *testing.T) {
	valid := emotionNameSet(builtinVocab())
	tests := []struct {
		name      string
		in        string
		wantClean string
		wantEmo   Expression
	}{
		{"no directive", "Hello there!", "Hello there!", ""},
		{"no directive preserves double space", "Hello  there", "Hello  there", ""},
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
			clean, emo := ParseEmotion(tt.in, valid)
			if clean != tt.wantClean {
				t.Errorf("clean = %q, want %q", clean, tt.wantClean)
			}
			if emo != tt.wantEmo {
				t.Errorf("emotion = %q, want %q", emo, tt.wantEmo)
			}
		})
	}
}

func TestParseEmotionCustomName(t *testing.T) {
	valid := emotionNameSet(BuildEmotionVocabulary(nil, []string{"grumpy"}, nil))
	clean, emo := ParseEmotion("[grumpy] go away", valid)
	if clean != "go away" || emo != Expression("grumpy") {
		t.Fatalf("clean=%q emo=%q, want %q/grumpy", clean, emo, "go away")
	}
	// A hyphenated custom name matches the widened token regex.
	valid2 := emotionNameSet(BuildEmotionVocabulary(nil, []string{"side-eye"}, nil))
	if _, e := ParseEmotion("[side-eye] hmm", valid2); e != Expression("side-eye") {
		t.Fatalf("hyphenated custom name not parsed: %q", e)
	}
	// A name not in the active vocabulary passes through untouched.
	if c, e := ParseEmotion("[grumpy] hi", emotionNameSet(builtinVocab())); c != "[grumpy] hi" || e != "" {
		t.Fatalf("unknown custom name should pass through: clean=%q emo=%q", c, e)
	}
}

func TestEmotionProtocolPrompt(t *testing.T) {
	p := emotionProtocolPrompt(builtinVocab())
	if !strings.Contains(p, "[happy]") {
		t.Errorf("protocol missing [happy] example: %q", p)
	}
	for _, e := range builtinVocab() {
		if !strings.Contains(p, e.Name) {
			t.Errorf("protocol missing vocabulary word %q", e.Name)
		}
	}
	if !strings.Contains(strings.ToLower(p), "never spoken") {
		t.Errorf("protocol must say the directive is never spoken: %q", p)
	}
}

func TestEmotionProtocolPromptDescriptions(t *testing.T) {
	p := emotionProtocolPrompt([]EmotionEntry{
		{Name: "grumpy", Description: "sulky and irritable"},
		{Name: "happy"},
	})
	if !strings.Contains(p, "grumpy — sulky and irritable") {
		t.Errorf("protocol missing described entry: %q", p)
	}
	if !strings.Contains(p, "happy") {
		t.Errorf("protocol missing bare entry: %q", p)
	}
}
