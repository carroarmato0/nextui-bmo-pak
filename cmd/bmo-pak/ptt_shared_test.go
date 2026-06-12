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

func TestSystemPromptWithContextThreeSegments(t *testing.T) {
	got := systemPromptWithContext("persona", "device", "remarks")
	if got != "persona\n\ndevice\n\nremarks" {
		t.Fatalf("three segments joined wrong: %q", got)
	}
}
