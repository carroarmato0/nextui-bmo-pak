package mod

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestLoadManifestFromFS(t *testing.T) {
	fsys := fstest.MapFS{
		"mod.json": {Data: []byte(`{"name":"Evil BMO","apiVersion":1}`)},
	}
	m := LoadManifest(fsys)
	if m.Name != "Evil BMO" {
		t.Errorf("Name = %q, want %q", m.Name, "Evil BMO")
	}
	if m.EffectiveAPIVersion() != 1 {
		t.Errorf("EffectiveAPIVersion = %d, want 1", m.EffectiveAPIVersion())
	}
}

func TestLoadManifestMissingReturnsZero(t *testing.T) {
	m := LoadManifest(fstest.MapFS{})
	if m.Name != "" {
		t.Errorf("Name = %q, want empty", m.Name)
	}
}

func TestLoadManifestAbsent(t *testing.T) {
	m := LoadManifest(os.DirFS(t.TempDir()))
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
	m := LoadManifest(os.DirFS(dir))
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
	m := LoadManifest(os.DirFS(dir))
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
	m := LoadManifest(os.DirFS(dir)) // must not panic; returns zero value
	if m.Name != "" || m.EffectiveAPIVersion() != 1 {
		t.Fatalf("malformed manifest should yield zero value, got %+v", m)
	}
}

func TestCurrentAPIVersionIsOne(t *testing.T) {
	if CurrentAPIVersion != 1 {
		t.Fatalf("CurrentAPIVersion = %d, want 1", CurrentAPIVersion)
	}
}

func TestLoadManifestEmotions(t *testing.T) {
	dir := t.TempDir()
	body := `{"name":"Evil BMO","emotions":{"grumpy":"sulky and irritable","ecstatic":"overjoyed"}}`
	if err := os.WriteFile(filepath.Join(dir, "mod.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	m := LoadManifest(os.DirFS(dir))
	if got := m.Emotions["grumpy"]; got != "sulky and irritable" {
		t.Fatalf("Emotions[grumpy] = %q", got)
	}
	if got := m.Emotions["ecstatic"]; got != "overjoyed" {
		t.Fatalf("Emotions[ecstatic] = %q", got)
	}
}

func TestLoadManifestNoEmotions(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "mod.json"), []byte(`{"name":"x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if m := LoadManifest(os.DirFS(dir)); m.Emotions != nil {
		t.Fatalf("Emotions should be nil when absent, got %v", m.Emotions)
	}
}

func TestLoadManifestAnimations(t *testing.T) {
	dir := t.TempDir()
	body := `{"name":"Evil","animations":{"speaking":{"frames":["m0","m1"],"driver":"amplitude"}}}`
	if err := os.WriteFile(filepath.Join(dir, "mod.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	m := LoadManifest(os.DirFS(dir))
	if _, ok := m.Animations["speaking"]; !ok {
		t.Fatal("speaking animation not parsed")
	}
}

func TestLoadManifestNoAnimations(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "mod.json"), []byte(`{"name":"x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if m := LoadManifest(os.DirFS(dir)); m.Animations != nil {
		t.Fatalf("expected nil animations, got %v", m.Animations)
	}
}
