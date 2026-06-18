package input

import (
	"context"
	"testing"
	"time"
)

func TestNavActionForKey(t *testing.T) {
	cases := []struct {
		code uint16
		want NavAction
		ok   bool
	}{
		{304, NavCancel, true},  // BTN_SOUTH = physical B — cancel/exit
		{305, NavConfirm, true}, // BTN_EAST = physical A — confirm/activate
		{314, 0, false},         // SELECT — unmapped
		{315, NavSave, true},
		{316, NavMenu, true},
		{btnDpadUp, NavUp, true},
		{btnDpadDown, NavDown, true},
		{btnDpadLeft, NavLeft, true},
		{btnDpadRight, NavRight, true},
		{307, NavGallery, true}, // BTN_NORTH = physical Y — next face / animation
		{308, NavQuote, true},   // BTN_WEST = physical X — speak a random quote
		{310, 0, false},         // L shoulder — no longer mapped
		{0, 0, false},
		{99, 0, false},
	}
	for _, tc := range cases {
		got, ok := navActionForKey(tc.code)
		if ok != tc.ok {
			t.Errorf("navActionForKey(%d) ok=%t, want %t", tc.code, ok, tc.ok)
			continue
		}
		if ok && got != tc.want {
			t.Errorf("navActionForKey(%d) = %v, want %v", tc.code, got, tc.want)
		}
	}
}

func TestNavActionForAbs(t *testing.T) {
	cases := []struct {
		code  uint16
		value int32
		want  NavAction
		ok    bool
	}{
		{absHat0X, -1, NavLeft, true},
		{absHat0X, 1, NavRight, true},
		{absHat0X, 0, 0, false},
		{absHat0Y, -1, NavUp, true},
		{absHat0Y, 1, NavDown, true},
		{absHat0Y, 0, 0, false},
		{99, 1, 0, false},
		{99, -1, 0, false},
	}
	for _, tc := range cases {
		got, ok := navActionForAbs(tc.code, tc.value)
		if ok != tc.ok {
			t.Errorf("navActionForAbs(%d,%d) ok=%t, want %t", tc.code, tc.value, ok, tc.ok)
			continue
		}
		if ok && got != tc.want {
			t.Errorf("navActionForAbs(%d,%d) = %v, want %v", tc.code, tc.value, got, tc.want)
		}
	}
}

// drainNav returns every NavAction currently buffered on the reader.
func drainNav(r *NavReader) []NavAction {
	var out []NavAction
	for {
		select {
		case a := <-r.events:
			out = append(out, a)
		default:
			return out
		}
	}
}

func TestNavReaderAutoRepeatWhileHeld(t *testing.T) {
	r, err := NewNavReader("dummy")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go r.repeater(ctx)

	r.setRepeat(NavDown)

	// Nothing should fire before the initial delay elapses.
	time.Sleep(repeatInitialDelay / 2)
	if got := drainNav(r); len(got) != 0 {
		t.Fatalf("got %d repeats before initial delay, want 0", len(got))
	}

	// Past the delay plus a few intervals, repeats should have accumulated.
	time.Sleep(repeatInitialDelay/2 + 3*repeatInterval + repeatInterval/2)
	got := drainNav(r)
	if len(got) < 3 {
		t.Fatalf("got %d repeats while held, want >=3", len(got))
	}
	for _, a := range got {
		if a != NavDown {
			t.Fatalf("repeat action = %v, want NavDown", a)
		}
	}

	// Releasing stops further repeats.
	r.clearRepeat(NavDown)
	time.Sleep(repeatInterval)
	drainNav(r)
	time.Sleep(3 * repeatInterval)
	if got := drainNav(r); len(got) != 0 {
		t.Fatalf("got %d repeats after release, want 0", len(got))
	}
}

func TestNavReaderAutoRepeatOnlyVertical(t *testing.T) {
	r, err := NewNavReader("dummy")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go r.repeater(ctx)

	// Left/right are not auto-repeated; holding one fires nothing on its own.
	r.setRepeat(NavLeft)
	time.Sleep(repeatInitialDelay + 3*repeatInterval)
	if got := drainNav(r); len(got) != 0 {
		t.Fatalf("got %d repeats for held left, want 0", len(got))
	}
}

func TestNewNavReaderEmptyPath(t *testing.T) {
	if _, err := NewNavReader(""); err == nil {
		t.Fatal("NewNavReader(\"\") = nil error, want error")
	}
	if _, err := NewNavReader("   "); err == nil {
		t.Fatal("NewNavReader(spaces) = nil error, want error")
	}
}
