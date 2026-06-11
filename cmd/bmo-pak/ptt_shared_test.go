package main

import "testing"

func TestSystemPromptWithContext(t *testing.T) {
	cases := []struct {
		persona, deviceCtx, want string
	}{
		{"", "", ""},
		{"be bmo", "", "be bmo"},
		{"", "DEVICE AWARENESS", "DEVICE AWARENESS"},
		{" be bmo \n", "\nDEVICE AWARENESS ", "be bmo\n\nDEVICE AWARENESS"},
	}
	for _, c := range cases {
		if got := systemPromptWithContext(c.persona, c.deviceCtx); got != c.want {
			t.Errorf("systemPromptWithContext(%q, %q) = %q, want %q", c.persona, c.deviceCtx, got, c.want)
		}
	}
}
