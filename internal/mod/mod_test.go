package mod

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestModPaths(t *testing.T) {
	m := Mod{ID: "evil", Root: "/x/mods/evil"}
	cases := map[string]string{
		m.PersonaPath(): "/x/mods/evil/persona.txt",
		m.VoicePath():   "/x/mods/evil/voice.txt",
		m.QuotesPath():  "/x/mods/evil/quotes.txt",
		m.FacesDir():    "/x/mods/evil/faces",
		m.AudioDir():    "/x/mods/evil/audio",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
	}
}

func TestFacesHasSVG(t *testing.T) {
	dir := t.TempDir()
	m := Mod{ID: "evil", Root: dir}
	if err := m.Open(nil); err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if m.FacesHasSVG() {
		t.Fatal("no faces dir yet: FacesHasSVG should be false")
	}
	facesDir := filepath.Join(dir, "faces")
	if err := os.MkdirAll(facesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(facesDir, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Re-open to pick up newly created faces dir
	m2 := Mod{ID: "evil", Root: dir}
	if err := m2.Open(nil); err != nil {
		t.Fatal(err)
	}
	defer m2.Close()
	if m2.FacesHasSVG() {
		t.Fatal("only a .txt present: FacesHasSVG should be false")
	}
	if err := os.WriteFile(filepath.Join(facesDir, "happy.svg"), []byte("<svg/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	m3 := Mod{ID: "evil", Root: dir}
	if err := m3.Open(nil); err != nil {
		t.Fatal(err)
	}
	defer m3.Close()
	if !m3.FacesHasSVG() {
		t.Fatal("an .svg is present: FacesHasSVG should be true")
	}
}

func TestSelfContained(t *testing.T) {
	dir := t.TempDir()
	facesDir := filepath.Join(dir, "faces")
	if err := os.MkdirAll(facesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(facesDir, "happy.svg"), []byte("<svg/>"), 0o644); err != nil {
		t.Fatal(err)
	}

	named := Mod{ID: "evil", Root: dir, IsDefault: false}
	if err := named.Open(nil); err != nil {
		t.Fatal(err)
	}
	defer named.Close()
	if !named.SelfContained() {
		t.Fatal("named mod with ≥1 svg must be self-contained")
	}

	def := Mod{ID: "default", Root: dir, IsDefault: true}
	if err := def.Open(nil); err != nil {
		t.Fatal(err)
	}
	defer def.Close()
	if def.SelfContained() {
		t.Fatal("default mod is overlay: never self-contained")
	}

	bare := Mod{ID: "lite", Root: t.TempDir(), IsDefault: false}
	if err := bare.Open(nil); err != nil {
		t.Fatal(err)
	}
	defer bare.Close()
	if bare.SelfContained() {
		t.Fatal("named mod with no faces must inherit embedded (not self-contained)")
	}
}

// writeZip writes a .zip at path whose entries are keyed by their in-archive
// path (e.g. "evil/mod.json"). Returns path for convenience.
func writeZip(t *testing.T, path string, files map[string]string) string {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return path
}

func TestOpenDirectoryMod(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "faces"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "faces", "neutral.svg"), []byte("<svg/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := Mod{ID: "evil", Root: dir}
	if err := m.Open(nil); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer m.Close()
	if !m.FacesHasSVG() {
		t.Error("FacesHasSVG() = false, want true")
	}
}

func TestOpenZipModWithWrappingFolder(t *testing.T) {
	dir := t.TempDir()
	zipPath := writeZip(t, filepath.Join(dir, "evil.zip"), map[string]string{
		"evil/mod.json":          `{"name":"Evil BMO"}`,
		"evil/faces/neutral.svg": "<svg/>",
	})
	m := Mod{ID: "evil", Root: zipPath}
	if err := m.Open(nil); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer m.Close()
	if !m.FacesHasSVG() {
		t.Error("FacesHasSVG() = false, want true (faces under evil/)")
	}
	if got := LoadManifest(m.FS).Name; got != "Evil BMO" {
		t.Errorf("manifest Name = %q, want %q", got, "Evil BMO")
	}
}

func TestOpenZipModRootFallback(t *testing.T) {
	dir := t.TempDir()
	var warned bool
	zipPath := writeZip(t, filepath.Join(dir, "evil.zip"), map[string]string{
		"mod.json":          `{"name":"Evil BMO"}`,
		"faces/neutral.svg": "<svg/>",
	})
	m := Mod{ID: "evil", Root: zipPath}
	if err := m.Open(func(string, ...any) { warned = true }); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer m.Close()
	if !m.FacesHasSVG() {
		t.Error("FacesHasSVG() = false, want true (faces at zip root)")
	}
	if !warned {
		t.Error("expected a warning for missing wrapping folder")
	}
}
