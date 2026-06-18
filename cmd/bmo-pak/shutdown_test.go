package main

import (
	"testing"
	"time"
)

func TestGoodbyeWait(t *testing.T) {
	cases := []struct {
		name    string
		clipDur time.Duration
		want    time.Duration
	}{
		{"no clip falls back to default", 0, 8 * time.Second},
		{"negative falls back to default", -1, 8 * time.Second},
		{"short clip gets a margin", 3 * time.Second, 5 * time.Second},
		{"the ~10s evil goodbye is heard in full", 10 * time.Second, 12 * time.Second},
		{"a pathologically long clip is capped", 5 * time.Minute, 30 * time.Second},
	}
	for _, c := range cases {
		if got := goodbyeWait(c.clipDur); got != c.want {
			t.Errorf("%s: goodbyeWait(%v) = %v, want %v", c.name, c.clipDur, got, c.want)
		}
	}
}
