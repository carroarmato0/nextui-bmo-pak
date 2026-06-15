package mod

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadManifestAbsent(t *testing.T) {
	m := LoadManifest(t.TempDir())
	if m.EffectiveAPIVersion() != 1 {
		t.Fatalf("absent manifest: EffectiveAPIVersion = %d, want 1", m.EffectiveAPIVersion())
	}
	if m.Name != "" {
		t.Fatalf("absent manifest: Name = %q, want empty", m.Name)
	}
}

func TestLoadManifestValid(t *testing.T) {
	dir := t.TempDir()
	body := `{"apiVersion":2,"name":"Evil BMO","author":"me","description":"d","version":"1.0"}`
	if err := os.WriteFile(filepath.Join(dir, "mod.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	m := LoadManifest(dir)
	if m.Name != "Evil BMO" || m.Author != "me" || m.Description != "d" || m.Version != "1.0" {
		t.Fatalf("manifest fields wrong: %+v", m)
	}
	if m.EffectiveAPIVersion() != 2 {
		t.Fatalf("EffectiveAPIVersion = %d, want 2", m.EffectiveAPIVersion())
	}
}

func TestLoadManifestPartialDefaultsAPIVersion(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "mod.json"), []byte(`{"name":"Just A Name"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	m := LoadManifest(dir)
	if m.Name != "Just A Name" {
		t.Fatalf("Name = %q", m.Name)
	}
	if m.EffectiveAPIVersion() != 1 {
		t.Fatalf("omitted apiVersion should default to 1, got %d", m.EffectiveAPIVersion())
	}
}

func TestLoadManifestMalformedTolerated(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "mod.json"), []byte(`{not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	m := LoadManifest(dir) // must not panic; returns zero value
	if m.Name != "" || m.EffectiveAPIVersion() != 1 {
		t.Fatalf("malformed manifest should yield zero value, got %+v", m)
	}
}

func TestCurrentAPIVersionIsOne(t *testing.T) {
	if CurrentAPIVersion != 1 {
		t.Fatalf("CurrentAPIVersion = %d, want 1", CurrentAPIVersion)
	}
}
