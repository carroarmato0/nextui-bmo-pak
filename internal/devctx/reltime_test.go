package devctx

import (
	"testing"
	"time"
)

func TestRelTime(t *testing.T) {
	now := time.Date(2026, 6, 11, 16, 0, 0, 0, time.UTC)
	cases := []struct {
		ago  time.Duration
		want string
	}{
		{30 * time.Second, "just now"},
		{90 * time.Second, "a minute ago"},
		{20 * time.Minute, "20 minutes ago"},
		{90 * time.Minute, "an hour ago"},
		{5 * time.Hour, "5 hours ago"},
		{30 * time.Hour, "yesterday"},
		{3 * 24 * time.Hour, "3 days ago"},
		{10 * 24 * time.Hour, "a week ago"},
		{20 * 24 * time.Hour, "2 weeks ago"},
		{45 * 24 * time.Hour, "a month ago"},
		{100 * 24 * time.Hour, "3 months ago"},
		{400 * 24 * time.Hour, "over a year ago"},
		{-time.Minute, "just now"}, // clock skew: never claim the future
	}
	for _, c := range cases {
		if got := RelTime(now.Add(-c.ago), now); got != c.want {
			t.Errorf("RelTime(now-%v) = %q, want %q", c.ago, got, c.want)
		}
	}
	if got := RelTime(time.Time{}, now); got != "" {
		t.Errorf("RelTime(zero) = %q, want empty", got)
	}
}
