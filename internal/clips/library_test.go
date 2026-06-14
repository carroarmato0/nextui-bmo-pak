package clips

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLibraryLoadEmbeddedHello(t *testing.T) {
	lib := NewLibrary(t.TempDir())
	data := lib.Load("hello")
	if len(data) == 0 {
		t.Fatal("expected embedded hello.pcm, got nil/empty")
	}
}

func TestLibraryLoadUnknownClipReturnsNil(t *testing.T) {
	lib := NewLibrary(t.TempDir())
	if got := lib.Load("does_not_exist"); got != nil {
		t.Fatalf("expected nil for unknown clip, got %d bytes", len(got))
	}
}

func TestLibraryOverridePreferredOverEmbedded(t *testing.T) {
	dir := t.TempDir()
	audioDir := filepath.Join(dir, "audio")
	if err := os.MkdirAll(audioDir, 0o755); err != nil {
		t.Fatal(err)
	}
	override := []byte{0x01, 0x02, 0x03, 0x04}
	if err := os.WriteFile(filepath.Join(audioDir, "hello.pcm"), override, 0o600); err != nil {
		t.Fatal(err)
	}
	lib := NewLibrary(dir)
	got := lib.Load("hello")
	if len(got) != len(override) || got[0] != override[0] {
		t.Fatalf("expected override bytes, got %v", got)
	}
}

func TestLibraryEmptyOverrideFallsBackToEmbedded(t *testing.T) {
	dir := t.TempDir()
	audioDir := filepath.Join(dir, "audio")
	if err := os.MkdirAll(audioDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Write an empty override file
	if err := os.WriteFile(filepath.Join(audioDir, "hello.pcm"), []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	lib := NewLibrary(dir)
	got := lib.Load("hello")
	// Should fall back to embedded (non-empty)
	if len(got) == 0 {
		t.Fatal("expected embedded fallback for empty override, got empty")
	}
}
