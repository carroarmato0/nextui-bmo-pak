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

func TestPlayLogCollectorDetailFull(t *testing.T) {
	now := time.Date(2026, 6, 11, 16, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "many_games.sqlite")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create tables
	for _, stmt := range []string{
		`CREATE TABLE rom(id INTEGER PRIMARY KEY, type TEXT, name TEXT, file_path TEXT, image_path TEXT, created_at INTEGER, updated_at INTEGER)`,
		`CREATE TABLE play_activity(rom_id INTEGER, play_time INTEGER, created_at INTEGER, updated_at INTEGER)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}

	// Insert 8 distinct games with sequential timestamps (oldest first)
	games := []string{"Game1", "Game2", "Game3", "Game4", "Game5", "Game6", "Game7", "Game8"}
	for i, name := range games {
		romID := i + 1
		if _, err := db.Exec(`INSERT INTO rom(id, name) VALUES (?, ?)`, romID, name); err != nil {
			t.Fatal(err)
		}
		// Each game has a single play session, with timestamps increasing from oldest to newest
		// Game1 is oldest (7 hours ago), Game8 is newest (just now)
		playTime := now.Add(time.Duration(-(7-i)) * time.Hour)
		if _, err := db.Exec(`INSERT INTO play_activity(rom_id, play_time, created_at) VALUES (?, ?, ?)`,
			romID, 3600, playTime.Unix()); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()

	// Test full mode (default) — should get all 8 games
	s, err := PlayLogCollector{DBPath: path}.Collect(now)
	if err != nil {
		t.Fatalf("collect (full mode): %v", err)
	}
	for _, game := range games {
		if !strings.Contains(s.Body, game) {
			t.Errorf("full mode missing game in body: %q not in %q", game, s.Body)
		}
	}

	// Test explicit full mode — should also get all 8 games
	s, err = PlayLogCollector{DBPath: path, Detail: "full"}.Collect(now)
	if err != nil {
		t.Fatalf("collect (detail=full): %v", err)
	}
	for _, game := range games {
		if !strings.Contains(s.Body, game) {
			t.Errorf("detail=full missing game in body: %q not in %q", game, s.Body)
		}
	}

	// Test random mode — should only get 5 most recent games
	// Most recent are: Game8, Game7, Game6, Game5, Game4 (the 5 newest timestamps)
	// Oldest are: Game1, Game2, Game3 (not in top 5)
	s2, err := PlayLogCollector{DBPath: path, Detail: "random"}.Collect(now)
	if err != nil {
		t.Fatalf("collect (detail=random): %v", err)
	}

	// Should contain the 5 most recent
	for _, game := range []string{"Game8", "Game7", "Game6", "Game5", "Game4"} {
		if !strings.Contains(s2.Body, game) {
			t.Errorf("random mode missing recent game: %q not in %q", game, s2.Body)
		}
	}

	// Should NOT contain the oldest 3
	for _, game := range []string{"Game1", "Game2", "Game3"} {
		if strings.Contains(s2.Body, game) {
			t.Errorf("random mode should not contain old game: %q found in %q", game, s2.Body)
		}
	}
}
