package renderer

import "testing"

func TestExprEpochResetsOnChange(t *testing.T) {
	var tr exprTracker
	// First observation of "neutral" at t=10 sets the start.
	if got := tr.epoch("neutral", 10.0); got != 0 {
		t.Fatalf("initial epoch=%v want 0", got)
	}
	// Same expr later -> elapsed since start.
	if got := tr.epoch("neutral", 13.5); got != 3.5 {
		t.Fatalf("epoch=%v want 3.5", got)
	}
	// Change expr -> resets.
	if got := tr.epoch("speaking", 14.0); got != 0 {
		t.Fatalf("epoch after change=%v want 0", got)
	}
	if got := tr.epoch("speaking", 16.0); got != 2.0 {
		t.Fatalf("epoch=%v want 2.0", got)
	}
}
