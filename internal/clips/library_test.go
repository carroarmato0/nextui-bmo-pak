package clips

import (
	"testing"
	"testing/fstest"
)

func TestLibraryLoadEmbeddedHello(t *testing.T) {
	lib := NewLibrary(nil)
	data := lib.Load("hello")
	if len(data) == 0 {
		t.Fatal("expected embedded hello.pcm, got nil/empty")
	}
}

func TestLibraryLoadUnknownClipReturnsNil(t *testing.T) {
	lib := NewLibrary(nil)
	if got := lib.Load("does_not_exist"); got != nil {
		t.Fatalf("expected nil for unknown clip, got %d bytes", len(got))
	}
}

func TestLibraryOverridePreferredOverEmbedded(t *testing.T) {
	override := []byte{0x01, 0x02, 0x03, 0x04}
	fsys := fstest.MapFS{
		"hello.pcm": {Data: override},
	}
	lib := NewLibrary(fsys)
	got := lib.Load("hello")
	if len(got) != len(override) || got[0] != override[0] {
		t.Fatalf("expected override bytes, got %v", got)
	}
}

func TestLibraryEmptyOverrideFallsBackToEmbedded(t *testing.T) {
	// An empty file in the FS (len 0) must fall back to the embedded asset.
	fsys := fstest.MapFS{
		"hello.pcm": {Data: []byte{}},
	}
	lib := NewLibrary(fsys)
	got := lib.Load("hello")
	// Should fall back to embedded (non-empty)
	if len(got) == 0 {
		t.Fatal("expected embedded fallback for empty override, got empty")
	}
}

func TestLibraryLoadsOverrideFromFS(t *testing.T) {
	fsys := fstest.MapFS{
		"hello.pcm": {Data: []byte("PCMDATA-1234")},
	}
	lib := NewLibrary(fsys)
	if got := lib.Load("hello"); string(got) != "PCMDATA-1234" {
		t.Errorf("Load = %q, want override bytes", got)
	}
}

func TestLibraryNilFSFallsBackToEmbedded(t *testing.T) {
	// hello.pcm is present in embedded assets/audio/
	lib := NewLibrary(nil)
	if got := lib.Load("hello"); got == nil {
		t.Error("Load(hello) = nil, want embedded default")
	}
}
