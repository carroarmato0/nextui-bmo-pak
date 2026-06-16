package face

import "testing"

func TestCanonical(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", ExprNeutral},
		{"idle", ExprNeutral},
		{"neutral", ExprNeutral},
		{" Neutral ", ExprNeutral},
		{"blink", ExprBlink},
		{"asleep", ExprSleeping},
		{"sleep", ExprSleeping},
		{"sleeping", ExprSleeping},
		{"error", ExprConcerned},
		{"confused", ExprConcerned},
		{"concerned", ExprConcerned},
		{"smile", ExprSmile},
		{"listening", ExprListening},
		{"thinking", ExprThinking},
		{"speaking", ExprNeutral}, // speaking folds to the templated neutral face
		{"look_around", ExprNeutral},
		// Reassigned: these no longer fold into smile/concerned.
		{"happy", ExprHappy},
		{"laugh", ExprNeutral}, // laugh dropped from vocabulary; folds to neutral
		{"excited", ExprExcited},
		{"sad", ExprSad},
		{"angry", ExprAngry},
		// New canonical names map to themselves.
		{"content", ExprContent},
		{"surprised", ExprSurprised},
		{"love", ExprLove},
		{"shy", ExprShy},
		{"crying", ExprCrying},
		{"teary", ExprTeary},
		{"gloomy", ExprGloomy},
		{"dizzy", ExprDizzy},
		{"unamused", ExprUnamused},
		{"annoyed", ExprAnnoyed},
		{"skeptical", ExprSkeptical},
		{"playful", ExprPlayful},
		{"kiss", ExprKiss},
		{"grimace", ExprGrimace},
		{"shout", ExprShout},
		{"dead", ExprDead},
		{"glitch", ExprGlitch},
		{"dismayed", ExprDismayed},
		{"adoring", ExprAdoring},
		{"sparkle", ExprSparkle},
		// A few new aliases.
		{"shocked", ExprSurprised},
		{"cry", ExprCrying},
		{"tongue", ExprPlayful},
		{"kissing", ExprKiss},
		{"sparkles", ExprSparkle},
	}
	for _, tc := range tests {
		if got := Canonical(tc.in); got != tc.want {
			t.Errorf("Canonical(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
