package main

import "fmt"

// faceLogger emits the active-face line once whenever the rendered expression
// changes, suppressing per-frame repeats. It is not safe for concurrent use;
// call it only from the render goroutine.
type faceLogger struct{ last string }

// note records expr and returns a formatted log line plus true when expr
// differs from the previously noted expression; otherwise it returns ("", false).
// source is "mod-override" / "embedded-default" / "none"; animated selects the
// "animated" vs "static" label.
func (f *faceLogger) note(expr, source string, animated bool) (string, bool) {
	if expr == f.last {
		return "", false
	}
	f.last = expr
	state := "static"
	if animated {
		state = "animated"
	}
	return fmt.Sprintf("face: rendering %q (%s, %s)", expr, source, state), true
}
