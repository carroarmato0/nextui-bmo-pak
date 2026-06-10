package input

import "testing"

func TestNavActionForKey(t *testing.T) {
	cases := []struct {
		code uint16
		want NavAction
		ok   bool
	}{
		{304, NavCancel, true}, // BTN_SOUTH = physical B — cancel/exit
		{305, 0, false},        // BTN_EAST = physical A — PTT only, no nav
		{314, 0, false},        // SELECT — unmapped
		{315, NavSave, true},
		{316, NavMenu, true},
		{btnDpadUp, NavUp, true},
		{btnDpadDown, NavDown, true},
		{btnDpadLeft, NavLeft, true},
		{btnDpadRight, NavRight, true},
		{307, 0, false}, // Y — no longer mapped
		{308, 0, false}, // X — no longer mapped
		{310, 0, false}, // L shoulder — no longer mapped
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

func TestNewNavReaderEmptyPath(t *testing.T) {
	if _, err := NewNavReader(""); err == nil {
		t.Fatal("NewNavReader(\"\") = nil error, want error")
	}
	if _, err := NewNavReader("   "); err == nil {
		t.Fatal("NewNavReader(spaces) = nil error, want error")
	}
}
