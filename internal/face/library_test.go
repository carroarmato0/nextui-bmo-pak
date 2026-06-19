package face

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestLibraryFallsBackToEmbedded(t *testing.T) {
	lib := NewLibrary(os.DirFS(filepath.Join(t.TempDir(), "does-not-exist")))
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
	lib := NewLibrary(os.DirFS(dir))
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
	lib := NewLibrary(os.DirFS(dir))
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
	lib := NewLibrary(os.DirFS(dir))
	_, fromDisk := lib.Bytes(ExprNeutral)
	if fromDisk {
		t.Fatal("blank override file must fall back to embedded")
	}
}

func TestLibraryPathTraversalBlocked(t *testing.T) {
	lib := NewLibrary(os.DirFS(t.TempDir()))
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
	lib := NewLibraryMode(os.DirFS(dir), true)

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
	lib := NewLibraryMode(os.DirFS(dir), true)
	if data, fromDisk := lib.Bytes("sad"); data != nil || fromDisk {
		t.Fatal("self-contained mod with no neutral must return (nil,false), not embedded")
	}
}

func TestNonSelfContainedStillFallsBackToEmbedded(t *testing.T) {
	lib := NewLibraryMode(os.DirFS(t.TempDir()), false)
	data, fromDisk := lib.Bytes(ExprNeutral)
	if fromDisk {
		t.Fatal("expected embedded source")
	}
	want, _ := defaultBytes(ExprNeutral)
	if string(data) != string(want) {
		t.Fatal("expected embedded neutral bytes")
	}
}

func TestResolveRawWhenFileExists(t *testing.T) {
	dir := t.TempDir()
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210"><rect width="280" height="210" fill="#0f0"/></svg>`
	if err := os.WriteFile(filepath.Join(dir, "grumpy.svg"), []byte(svg), 0o644); err != nil {
		t.Fatal(err)
	}
	lib := NewLibraryMode(os.DirFS(dir), true)
	if got := lib.Resolve("grumpy"); got != "grumpy" {
		t.Fatalf("Resolve(grumpy) = %q, want grumpy", got)
	}
	// No happy.svg on disk: falls back to the canonical name.
	if got := lib.Resolve("happy"); got != "happy" {
		t.Fatalf("Resolve(happy) = %q, want happy", got)
	}
	// Alias with no disk file resolves through Canonical.
	if got := lib.Resolve("shocked"); got != ExprSurprised {
		t.Fatalf("Resolve(shocked) = %q, want %q", got, ExprSurprised)
	}
}

func TestResolveCanonicalWhenNoDir(t *testing.T) {
	lib := NewLibrary(os.DirFS(filepath.Join(t.TempDir(), "missing")))
	if got := lib.Resolve("cry"); got != ExprCrying {
		t.Fatalf("Resolve(cry) = %q, want %q", got, ExprCrying)
	}
	// Unsafe names never hit disk and fold to neutral via Canonical.
	if got := lib.Resolve("../etc/passwd"); got != ExprNeutral {
		t.Fatalf("Resolve(traversal) = %q, want %q", got, ExprNeutral)
	}
}

const sourceTestSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210"><rect width="280" height="210" fill="#123"/></svg>`

func TestSourceEmbeddedDefault(t *testing.T) {
	lib := NewLibrary(os.DirFS(filepath.Join(t.TempDir(), "missing")))
	if got := lib.Source(ExprNeutral); got != "embedded-default" {
		t.Fatalf("Source = %q, want embedded-default", got)
	}
}

func TestSourceModOverride(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "neutral.svg"), []byte(sourceTestSVG), 0o644); err != nil {
		t.Fatal(err)
	}
	lib := NewLibrary(os.DirFS(dir))
	if got := lib.Source("neutral"); got != "mod-override" {
		t.Fatalf("Source = %q, want mod-override", got)
	}
}

func TestSourceOverrideViaAlias(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "crying.svg"), []byte(sourceTestSVG), 0o644); err != nil {
		t.Fatal(err)
	}
	lib := NewLibrary(os.DirFS(dir))
	if got := lib.Source("cry"); got != "mod-override" {
		t.Fatalf("Source = %q, want mod-override", got)
	}
}

func TestSourceSelfContainedFoldsToModNeutral(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "neutral.svg"), []byte(sourceTestSVG), 0o644); err != nil {
		t.Fatal(err)
	}
	lib := NewLibraryMode(os.DirFS(dir), true)
	if got := lib.Source("sad"); got != "mod-override" {
		t.Fatalf("Source = %q, want mod-override (folded to mod neutral)", got)
	}
}

func TestSourceSelfContainedNoFaceIsNone(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "happy.svg"), []byte(sourceTestSVG), 0o644); err != nil {
		t.Fatal(err)
	}
	lib := NewLibraryMode(os.DirFS(dir), true)
	if got := lib.Source("sad"); got != "none" {
		t.Fatalf("Source = %q, want none", got)
	}
}

func TestSourceBlankOverrideFallsBack(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "neutral.svg"), []byte("   \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lib := NewLibrary(os.DirFS(dir))
	if got := lib.Source(ExprNeutral); got != "embedded-default" {
		t.Fatalf("Source = %q, want embedded-default (blank ignored)", got)
	}
}

func TestLibraryReadsOverrideFromFS(t *testing.T) {
	fsys := fstest.MapFS{
		"neutral.svg": {Data: []byte("<svg id=\"override\"/>")},
	}
	lib := NewLibraryMode(fsys, true)
	data, fromDisk := lib.Bytes("neutral")
	if !fromDisk {
		t.Fatal("expected override to come from fsys")
	}
	if string(data) != "<svg id=\"override\"/>" {
		t.Errorf("got %q", data)
	}
}

func TestFaceNamesInFS(t *testing.T) {
	fsys := fstest.MapFS{
		"neutral.svg": {Data: []byte("<svg/>")},
		"angry.svg":   {Data: []byte("<svg/>")},
		"notes.txt":   {Data: []byte("ignore me")},
	}
	got := FaceNamesInFS(fsys)
	if len(got) != 2 || got[0] != "angry" || got[1] != "neutral" {
		t.Errorf("FaceNamesInFS = %v, want [angry neutral]", got)
	}
}
