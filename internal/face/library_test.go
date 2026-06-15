package face

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLibraryFallsBackToEmbedded(t *testing.T) {
	lib := NewLibrary(filepath.Join(t.TempDir(), "does-not-exist"))
	data, fromDisk := lib.Bytes(ExprNeutral)
	if fromDisk {
		t.Fatal("expected embedded source")
	}
	want, _ := defaultBytes(ExprNeutral)
	if string(data) != string(want) {
		t.Fatal("expected embedded neutral bytes")
	}
}

func TestLibraryDiskOverrideWins(t *testing.T) {
	dir := t.TempDir()
	custom := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210"><rect width="280" height="210" fill="#000"/></svg>`
	if err := os.WriteFile(filepath.Join(dir, "neutral.svg"), []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	lib := NewLibrary(dir)
	data, fromDisk := lib.Bytes("neutral")
	if !fromDisk || string(data) != custom {
		t.Fatalf("expected disk override, fromDisk=%v", fromDisk)
	}
}

func TestLibraryAliasResolution(t *testing.T) {
	dir := t.TempDir()
	custom := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210"><rect width="280" height="210" fill="#111"/></svg>`
	// Write under the canonical name "crying", but look up via alias "cry"
	if err := os.WriteFile(filepath.Join(dir, "crying.svg"), []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	lib := NewLibrary(dir)
	data, fromDisk := lib.Bytes("cry")
	if !fromDisk || string(data) != custom {
		t.Fatalf("expected disk override for alias, fromDisk=%v", fromDisk)
	}
}

func TestLibraryBlankFileIgnored(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "neutral.svg"), []byte("   \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lib := NewLibrary(dir)
	_, fromDisk := lib.Bytes(ExprNeutral)
	if fromDisk {
		t.Fatal("blank override file must fall back to embedded")
	}
}

func TestLibraryPathTraversalBlocked(t *testing.T) {
	lib := NewLibrary(t.TempDir())
	if _, fromDisk := lib.Bytes("../../etc/passwd"); fromDisk {
		t.Fatal("path traversal must not read from disk")
	}
}
