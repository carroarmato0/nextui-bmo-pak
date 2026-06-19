package mod

import (
	"os"
	"path/filepath"
	"testing"
)

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func ids(mods []Mod) []string {
	out := make([]string, len(mods))
	for i, m := range mods {
		out[i] = m.ID
	}
	return out
}

func TestDiscoverEmptyRootHasDefaultOnly(t *testing.T) {
	mods := Discover(filepath.Join(t.TempDir(), "mods"), nil) // dir doesn't exist
	if len(mods) != 1 {
		t.Fatalf("want 1 mod (synthetic default), got %d", len(mods))
	}
	if !mods[0].IsDefault || mods[0].ID != DefaultID {
		t.Fatalf("first entry must be the default, got %+v", mods[0])
	}
}

func TestDiscoverOrdersDefaultFirstThenAlpha(t *testing.T) {
	root := filepath.Join(t.TempDir(), "mods")
	mkdir(t, filepath.Join(root, "zebra"))
	mkdir(t, filepath.Join(root, "alpha"))
	mkdir(t, filepath.Join(root, "default"))
	// Noise that must be ignored:
	mkdir(t, filepath.Join(root, ".git"))
	if err := os.WriteFile(filepath.Join(root, "loose.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	mods := Discover(root, nil)
	var got []string
	for _, m := range mods {
		got = append(got, m.ID)
	}
	want := []string{DefaultID, "alpha", "zebra"}
	if len(got) != len(want) {
		t.Fatalf("ids = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ids = %v, want %v", got, want)
		}
	}
	if !mods[0].IsDefault {
		t.Fatal("default must be first and flagged IsDefault")
	}
}

func TestDiscoverAttachesManifest(t *testing.T) {
	root := filepath.Join(t.TempDir(), "mods")
	evil := filepath.Join(root, "evil")
	mkdir(t, evil)
	if err := os.WriteFile(filepath.Join(evil, "mod.json"), []byte(`{"name":"Evil BMO"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, m := range Discover(root, nil) {
		if m.ID == "evil" {
			if m.DisplayName() != "Evil BMO" {
				t.Fatalf("DisplayName = %q, want %q", m.DisplayName(), "Evil BMO")
			}
			return
		}
	}
	t.Fatal("evil mod not discovered")
}

func TestActiveFallsBackToDefault(t *testing.T) {
	root := filepath.Join(t.TempDir(), "mods")
	mkdir(t, filepath.Join(root, "evil"))
	mods := Discover(root, nil)

	if got := Active(mods, "evil"); got.ID != "evil" {
		t.Fatalf("Active(evil) = %q, want evil", got.ID)
	}
	if got := Active(mods, ""); !got.IsDefault {
		t.Fatal("Active(\"\") must return the default entry")
	}
	if got := Active(mods, "ghost"); !got.IsDefault {
		t.Fatal("Active(unknown id) must fall back to the default entry")
	}
}

func TestDiscoverFindsZipMod(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "default"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeZip(t, filepath.Join(root, "evil.zip"), map[string]string{
		"evil/mod.json": `{"name":"Evil BMO"}`,
	})
	mods := Discover(root, nil)
	if got := ids(mods); len(got) != 2 || got[0] != "default" || got[1] != "evil" {
		t.Fatalf("ids = %v, want [default evil]", got)
	}
	if mods[1].Manifest.Name != "Evil BMO" {
		t.Errorf("zip manifest Name = %q, want %q", mods[1].Manifest.Name, "Evil BMO")
	}
	if mods[1].Root != filepath.Join(root, "evil.zip") {
		t.Errorf("Root = %q, want the .zip path", mods[1].Root)
	}
}

func TestDiscoverDirectoryWinsOverZip(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "default"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "evil"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeZip(t, filepath.Join(root, "evil.zip"), map[string]string{"evil/mod.json": `{}`})

	var warned bool
	mods := Discover(root, func(string, ...any) { warned = true })

	var evil *Mod
	for i := range mods {
		if mods[i].ID == "evil" {
			evil = &mods[i]
		}
	}
	if evil == nil {
		t.Fatal("evil mod not found")
	}
	if evil.Root != filepath.Join(root, "evil") {
		t.Errorf("Root = %q, want the directory (dir wins)", evil.Root)
	}
	if !warned {
		t.Error("expected a warning when both directory and .zip exist")
	}
	count := 0
	for _, id := range ids(mods) {
		if id == "evil" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("evil appears %d times, want 1", count)
	}
}

func TestOpenCorruptZipReturnsError(t *testing.T) {
	root := t.TempDir()
	zipPath := filepath.Join(root, "broken.zip")
	if err := os.WriteFile(zipPath, []byte("not a real zip"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := Mod{ID: "broken", Root: zipPath}
	if err := m.Open(nil); err == nil {
		m.Close()
		t.Fatal("Open(corrupt zip) = nil error, want error")
	}
}

func TestDiscoverListsCorruptZipByID(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "default"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "broken.zip"), []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	mods := Discover(root, nil) // manifestFor swallows the open error → zero manifest
	found := false
	for _, m := range mods {
		if m.ID == "broken" {
			found = true
		}
	}
	if !found {
		t.Error("corrupt zip should still be listed by id")
	}
}
