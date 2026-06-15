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

func TestDiscoverEmptyRootHasDefaultOnly(t *testing.T) {
	mods := Discover(filepath.Join(t.TempDir(), "mods")) // dir doesn't exist
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

	mods := Discover(root)
	var ids []string
	for _, m := range mods {
		ids = append(ids, m.ID)
	}
	want := []string{DefaultID, "alpha", "zebra"}
	if len(ids) != len(want) {
		t.Fatalf("ids = %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("ids = %v, want %v", ids, want)
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
	for _, m := range Discover(root) {
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
	mods := Discover(root)

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
