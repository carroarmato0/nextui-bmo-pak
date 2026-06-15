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

func TestSelfContainedFoldsMissingToModNeutral(t *testing.T) {
	dir := t.TempDir()
	happy := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210"><rect width="280" height="210" fill="#0f0"/></svg>`
	neutral := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210"><rect width="280" height="210" fill="#00f"/></svg>`
	if err := os.WriteFile(filepath.Join(dir, "happy.svg"), []byte(happy), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "neutral.svg"), []byte(neutral), 0o644); err != nil {
		t.Fatal(err)
	}
	lib := NewLibraryMode(dir, true)

	// An expression the mod ships is served directly.
	if data, fromDisk := lib.Bytes("happy"); !fromDisk || string(data) != happy {
		t.Fatalf("happy: fromDisk=%v", fromDisk)
	}
	// A missing expression folds to the mod's own neutral, never embedded.
	data, fromDisk := lib.Bytes("sad")
	if !fromDisk || string(data) != neutral {
		t.Fatalf("missing expr should fold to mod neutral, fromDisk=%v", fromDisk)
	}
}

func TestSelfContainedNoNeutralReturnsNothing(t *testing.T) {
	dir := t.TempDir()
	happy := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210"><rect width="280" height="210" fill="#0f0"/></svg>`
	if err := os.WriteFile(filepath.Join(dir, "happy.svg"), []byte(happy), 0o644); err != nil {
		t.Fatal(err)
	}
	lib := NewLibraryMode(dir, true)
	if data, fromDisk := lib.Bytes("sad"); data != nil || fromDisk {
		t.Fatal("self-contained mod with no neutral must return (nil,false), not embedded")
	}
}

func TestNonSelfContainedStillFallsBackToEmbedded(t *testing.T) {
	lib := NewLibraryMode(t.TempDir(), false)
	data, fromDisk := lib.Bytes(ExprNeutral)
	if fromDisk {
		t.Fatal("expected embedded source")
	}
	want, _ := defaultBytes(ExprNeutral)
	if string(data) != string(want) {
		t.Fatal("expected embedded neutral bytes")
	}
}
