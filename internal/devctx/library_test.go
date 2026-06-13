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
	if !strings.Contains(s.Body, "2 platforms, 4 total titles") {
		t.Errorf("missing totals in body: %q", s.Body)
	}
	// Sorted by count descending: GB (3) before MD (1).
	gb := strings.Index(s.Body, "Game Boy: a, b, c")
	md := strings.Index(s.Body, "Sega Genesis: x")
	if gb == -1 || md == -1 || gb > md {
		t.Errorf("expected count-sorted platforms with title lists in body: %q", s.Body)
	}
	if strings.Contains(s.Body, "Virtual Boy") || strings.Contains(s.Body, ".res") {
		t.Errorf("empty/hidden dirs leaked into body: %q", s.Body)
	}
	if !s.Freshest.IsZero() {
		t.Errorf("library is evergreen; Freshest must be zero, got %v", s.Freshest)
	}
}

func TestLibraryCollectorVariantGrouping(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, filepath.Join(root, "Game Boy Advance (GBA)"), "pokemon.gba", "zelda.gba")
	writeFiles(t, filepath.Join(root, "Game Boy Advance (MGBA)"), "zelda.gba", "metroid.gba")

	s, err := LibraryCollector{Root: root}.Collect(time.Now())
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if !strings.Contains(s.Body, "1 platforms, 3 total titles") {
		t.Errorf("expected 1 platform with 3 deduplicated titles: %q", s.Body)
	}
	if !strings.Contains(s.Body, "Game Boy Advance: metroid, pokemon, zelda") {
		t.Errorf("expected sorted deduplicated title list: %q", s.Body)
	}
}

func TestLibraryCollectorDetailRandom(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, filepath.Join(root, "Super Nintendo (SFC)"), "a.sfc", "b.sfc", "c.sfc", "d.sfc", "e.sfc")

	s, err := LibraryCollector{Root: root, Detail: "random"}.Collect(time.Now())
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if !strings.Contains(s.Body, "1 platforms, 5 total titles") {
		t.Errorf("missing totals: %q", s.Body)
	}
	if !strings.Contains(s.Body, "Super Nintendo (5 titles): e.g.") {
		t.Errorf("expected random-mode format: %q", s.Body)
	}
	// In random mode the title list should NOT be comma-separated inline.
	// The platform line should only show one example title, not all five.
	lines := strings.Split(s.Body, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "Super Nintendo") {
			parts := strings.Split(line, ", ")
			if len(parts) > 1 {
				t.Errorf("random mode should not produce comma-separated title list: %q", line)
			}
		}
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
