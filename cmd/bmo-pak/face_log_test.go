package main

import "testing"

func TestFaceLogNoteFormatsLine(t *testing.T) {
	var f faceLogger
	line, ok := f.note("happy", "embedded-default", true)
	if !ok {
		t.Fatal("first note should log")
	}
	if want := `face: rendering "happy" (embedded-default, animated)`; line != want {
		t.Fatalf("line = %q, want %q", line, want)
	}
}

func TestFaceLogNoteStaticLabel(t *testing.T) {
	var f faceLogger
	line, ok := f.note("neutral", "mod-override", false)
	if !ok {
		t.Fatal("first note should log")
	}
	if want := `face: rendering "neutral" (mod-override, static)`; line != want {
		t.Fatalf("line = %q, want %q", line, want)
	}
}

func TestFaceLogNoteSuppressesRepeat(t *testing.T) {
	var f faceLogger
	f.note("happy", "embedded-default", true)
	line, ok := f.note("happy", "embedded-default", true)
	if ok || line != "" {
		t.Fatalf("repeat should be suppressed, got (%q, %v)", line, ok)
	}
}

func TestFaceLogNoteRelogsOnChange(t *testing.T) {
	var f faceLogger
	if _, ok := f.note("happy", "embedded-default", true); !ok {
		t.Fatal("happy should log")
	}
	if _, ok := f.note("sad", "embedded-default", false); !ok {
		t.Fatal("sad should log")
	}
	if _, ok := f.note("sad", "embedded-default", false); ok {
		t.Fatal("repeated sad should be suppressed")
	}
	if _, ok := f.note("happy", "embedded-default", true); !ok {
		t.Fatal("change-back to happy should re-log")
	}
}

func TestFaceLogNoteFirstEverLogs(t *testing.T) {
	var f faceLogger
	if _, ok := f.note("neutral", "embedded-default", false); !ok {
		t.Fatal("first note of zero-value logger should log")
	}
}
