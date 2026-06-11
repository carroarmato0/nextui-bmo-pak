package devctx

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixtureDB(t *testing.T, now time.Time) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "game_logs.sqlite")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	stmts := []string{
		`CREATE TABLE rom(id INTEGER PRIMARY KEY, type TEXT, name TEXT, file_path TEXT, image_path TEXT, created_at INTEGER, updated_at INTEGER)`,
		`CREATE TABLE play_activity(rom_id INTEGER, play_time INTEGER, created_at INTEGER, updated_at INTEGER)`,
		`INSERT INTO rom(id, name) VALUES (1, 'Deadeus'), (2, 'Deadeus'), (3, 'Splore'), (4, 'Crash Bandicoot (USA)')`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
	insert := func(romID int, playSeconds int64, at time.Time) {
		if _, err := db.Exec(`INSERT INTO play_activity(rom_id, play_time, created_at) VALUES (?, ?, ?)`,
			romID, playSeconds, at.Unix()); err != nil {
			t.Fatal(err)
		}
	}
	insert(1, 4634, now.Add(-2*time.Hour))     // Deadeus rom A
	insert(2, 286, now.Add(-26*time.Hour))     // Deadeus rom B (same name)
	insert(3, 13597, now.Add(-3*24*time.Hour)) // Splore, most played
	insert(4, 164, now.Add(-27*time.Hour))     // Crash
	return path
}

func TestPlayLogCollector(t *testing.T) {
	now := time.Date(2026, 6, 11, 16, 0, 0, 0, time.UTC)
	s, err := PlayLogCollector{DBPath: fixtureDB(t, now)}.Collect(now)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if s.Key != KeyPlayLog || s.Title != "PLAY HISTORY" {
		t.Fatalf("unexpected section identity: %+v", s)
	}
	if !strings.Contains(s.Body, "Last play session was 2 hours ago") {
		t.Errorf("missing reunion gap line: %q", s.Body)
	}
	// Same-name rom rows merged: 4634+286 = 4920s = 1h22m total.
	if !strings.Contains(s.Body, "Deadeus (last played 2 hours ago, 1h22m total)") {
		t.Errorf("missing merged recent entry: %q", s.Body)
	}
	// Most played ordering: Splore (3h46m) before Deadeus (1h22m).
	sp := strings.Index(s.Body, "Splore (3h46m total)")
	de := strings.LastIndex(s.Body, "Deadeus (1h22m total)")
	if sp == -1 || de == -1 || sp > de {
		t.Errorf("most-played list wrong: %q", s.Body)
	}
	if !s.Freshest.Equal(now.Add(-2 * time.Hour)) {
		t.Errorf("Freshest = %v, want most recent session", s.Freshest)
	}
}

func TestPlayLogCollectorMissingDB(t *testing.T) {
	if _, err := (PlayLogCollector{DBPath: "/nonexistent/db.sqlite"}).Collect(time.Now()); err == nil {
		t.Fatal("expected error for missing db")
	}
}

func TestPlayLogCollectorEmptyDB(t *testing.T) {
	now := time.Now()
	path := filepath.Join(t.TempDir(), "empty.sqlite")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`CREATE TABLE rom(id INTEGER PRIMARY KEY, name TEXT)`,
		`CREATE TABLE play_activity(rom_id INTEGER, play_time INTEGER, created_at INTEGER)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	if _, err := (PlayLogCollector{DBPath: path}).Collect(now); err == nil {
		t.Fatal("expected error for empty play log")
	}
}
