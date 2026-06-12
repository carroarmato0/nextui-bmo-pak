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
		{"angry", ExprConcerned},
		{"sad", ExprConcerned},
		{"concerned", ExprConcerned},
		{"happy", ExprSmile},
		{"smile", ExprSmile},
		{"laugh", ExprSmile},
		{"excited", ExprSmile},
		{"listening", ExprListening},
		{"thinking", ExprThinking},
		{"speaking", ExprSpeaking},
		{"look_around", ExprNeutral},
	}
	for _, tc := range tests {
		if got := Canonical(tc.in); got != tc.want {
			t.Errorf("Canonical(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
