package devctx

import (
	"fmt"
	"time"
)

// RelTime renders t relative to now ("just now", "20 minutes ago",
// "yesterday", "2 weeks ago"). Chat models reason far better about
// pre-digested relative phrases than raw timestamps, so collectors must
// never emit epochs. Zero time renders as "".
func RelTime(t, now time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < 2*time.Minute:
		return "a minute ago"
	case d < time.Hour:
		return fmt.Sprintf("%d minutes ago", int(d.Minutes()))
	case d < 2*time.Hour:
		return "an hour ago"
	case d < 24*time.Hour:
		return fmt.Sprintf("%d hours ago", int(d.Hours()))
	case d < 48*time.Hour:
		return "yesterday"
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%d days ago", int(d.Hours()/24))
	case d < 14*24*time.Hour:
		return "a week ago"
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%d weeks ago", int(d.Hours()/(24*7)))
	case d < 60*24*time.Hour:
		return "a month ago"
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%d months ago", int(d.Hours()/(24*30)))
	default:
		return "over a year ago"
	}
}
