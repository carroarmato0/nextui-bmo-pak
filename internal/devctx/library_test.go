package devctx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeFiles(t *testing.T, dir string, names ...string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLibraryCollectorCountsPerSystem(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, filepath.Join(root, "Game Boy (GB)"), "a.gb", "b.gb", "c.gb")
	writeFiles(t, filepath.Join(root, "Sega Genesis (MD)"), "x.md")
	writeFiles(t, filepath.Join(root, "Virtual Boy (VB)"))         // empty: skipped
	writeFiles(t, filepath.Join(root, ".res"), "hidden.png")       // hidden dir: skipped
	writeFiles(t, filepath.Join(root, "Game Boy (GB)"), ".hidden") // hidden file: not counted
	if err := os.WriteFile(filepath.Join(root, "recentlist.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err) // stray file at root: ignored
	}

	s, err := LibraryCollector{Root: root}.Collect(time.Now())
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if s.Key != KeyLibrary || s.Title != "GAME LIBRARY" {
		t.Fatalf("unexpected section identity: %+v", s)
	}
	if !strings.Contains(s.Body, "4 games across 2 systems") {
		t.Errorf("missing totals in body: %q", s.Body)
	}
	// Sorted by count descending: GB before MD.
	gb := strings.Index(s.Body, "Game Boy (GB): 3")
	md := strings.Index(s.Body, "Sega Genesis (MD): 1")
	if gb == -1 || md == -1 || gb > md {
		t.Errorf("expected count-sorted systems in body: %q", s.Body)
	}
	if strings.Contains(s.Body, "Virtual Boy") || strings.Contains(s.Body, ".res") {
		t.Errorf("empty/hidden dirs leaked into body: %q", s.Body)
	}
	if !s.Freshest.IsZero() {
		t.Errorf("library is evergreen; Freshest must be zero, got %v", s.Freshest)
	}
}

func TestLibraryCollectorMissingRoot(t *testing.T) {
	if _, err := (LibraryCollector{Root: "/nonexistent/roms"}).Collect(time.Now()); err == nil {
		t.Fatal("expected error for missing root")
	}
}

func TestLibraryCollectorEmptyLibrary(t *testing.T) {
	if _, err := (LibraryCollector{Root: t.TempDir()}).Collect(time.Now()); err == nil {
		t.Fatal("expected error for empty library")
	}
}
