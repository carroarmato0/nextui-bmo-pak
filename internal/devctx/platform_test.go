package devctx

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBaseName(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"Game Boy Advance (GBA)", "Game Boy Advance"},
		{"Game Boy Advance (MGBA)", "Game Boy Advance"},
		{"Super Nintendo Entertainment System (SFC)", "Super Nintendo Entertainment System"},
		{"GB", "GB"},
		{"Pico-8 (P8)", "Pico-8"},
		{"Game Boy (GB)", "Game Boy"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := baseName(tc.input)
			if got != tc.want {
				t.Errorf("baseName(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// writeFile creates a file with the given content under path, creating
// intermediate directories as needed.
func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestPlatformGroups(t *testing.T) {
	root := t.TempDir()

	// Game Boy Advance (GBA): 2 files
	writeFile(t, filepath.Join(root, "Game Boy Advance (GBA)", "Pokemon Red (USA).gba"))
	writeFile(t, filepath.Join(root, "Game Boy Advance (GBA)", "Castlevania.gba"))
	// Game Boy Advance (MGBA): 3 files (one same name, two unique)
	writeFile(t, filepath.Join(root, "Game Boy Advance (MGBA)", "Pokemon Red (USA).gba"))
	writeFile(t, filepath.Join(root, "Game Boy Advance (MGBA)", "Metroid Fusion.gba"))
	writeFile(t, filepath.Join(root, "Game Boy Advance (MGBA)", "Golden Sun.gba"))

	// Pico-8: 1 file
	writeFile(t, filepath.Join(root, "Pico-8 (P8)", "celeste.p8"))

	// Hidden dir — must be excluded
	if err := os.MkdirAll(filepath.Join(root, ".hidden"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Empty group — must be excluded
	if err := os.MkdirAll(filepath.Join(root, "Empty System (ES)"), 0o755); err != nil {
		t.Fatal(err)
	}

	groups, err := platformGroups(root)
	if err != nil {
		t.Fatalf("platformGroups: %v", err)
	}

	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d: %v", len(groups), groups)
	}

	// First group: Game Boy Advance (5 files total across 2 dirs)
	gba := groups[0]
	if gba.Name != "Game Boy Advance" {
		t.Errorf("groups[0].Name = %q, want %q", gba.Name, "Game Boy Advance")
	}
	if len(gba.Dirs) != 2 {
		t.Errorf("groups[0].Dirs len = %d, want 2", len(gba.Dirs))
	}

	// Second group: Pico-8 (1 file)
	p8 := groups[1]
	if p8.Name != "Pico-8" {
		t.Errorf("groups[1].Name = %q, want %q", p8.Name, "Pico-8")
	}
	if len(p8.Dirs) != 1 {
		t.Errorf("groups[1].Dirs len = %d, want 1", len(p8.Dirs))
	}
}

func TestGameName(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"Pokemon Red (USA).zip.sav", "Pokemon Red (USA)"},
		{"Chrono Trigger.smc", "Chrono Trigger"},
		{"game.gb", "game"},
		{"noext", "noext"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := gameName(tc.input)
			if got != tc.want {
				t.Errorf("gameName(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
