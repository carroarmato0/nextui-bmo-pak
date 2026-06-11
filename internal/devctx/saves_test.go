package devctx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeSave(t *testing.T, path string, mtime time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

func TestSavesCollector(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 6, 11, 16, 0, 0, 0, time.UTC)
	writeSave(t, filepath.Join(root, "GB", "Deadeus.gb.sav"), now.Add(-2*time.Hour))
	writeSave(t, filepath.Join(root, "GB", "Pokemon - Red Version (USA, Europe).zip.sav"), now.Add(-72*time.Hour))
	writeSave(t, filepath.Join(root, "PS", "Crash Bandicoot (USA).cue.sav"), now.Add(-30*time.Hour))

	s, err := SavesCollector{Root: root}.Collect(now)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if s.Key != KeySaves || s.Title != "SAVE FILES" {
		t.Fatalf("unexpected section identity: %+v", s)
	}
	if !strings.Contains(s.Body, "3 save files") {
		t.Errorf("missing total: %q", s.Body)
	}
	if !strings.Contains(s.Body, "GB: 2") || !strings.Contains(s.Body, "PS: 1") {
		t.Errorf("missing per-system counts: %q", s.Body)
	}
	// Double extensions stripped so real game titles surface.
	if !strings.Contains(s.Body, "Deadeus (GB, 2 hours ago)") {
		t.Errorf("missing recent save with relative time: %q", s.Body)
	}
	if strings.Contains(s.Body, ".sav") || strings.Contains(s.Body, ".zip") {
		t.Errorf("extensions leaked into body: %q", s.Body)
	}
	// Recency order: Deadeus (2h) before Crash (30h) before Pokemon (72h).
	d := strings.Index(s.Body, "Deadeus")
	cr := strings.Index(s.Body, "Crash Bandicoot")
	p := strings.Index(s.Body, "Pokemon")
	if !(d < cr && cr < p) {
		t.Errorf("recent saves not sorted by mtime desc: %q", s.Body)
	}
	if !s.Freshest.Equal(now.Add(-2 * time.Hour)) {
		t.Errorf("Freshest = %v, want newest save mtime", s.Freshest)
	}
}

func TestSavesCollectorEmpty(t *testing.T) {
	if _, err := (SavesCollector{Root: t.TempDir()}).Collect(time.Now()); err == nil {
		t.Fatal("expected error for empty saves dir")
	}
}
