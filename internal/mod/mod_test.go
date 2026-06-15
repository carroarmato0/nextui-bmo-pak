package mod

import (
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
		m.AudioDir():    "/x/mods/evil",
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
	if m.FacesHasSVG() {
		t.Fatal("only a .txt present: FacesHasSVG should be false")
	}
	if err := os.WriteFile(filepath.Join(facesDir, "happy.svg"), []byte("<svg/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !m.FacesHasSVG() {
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
	if !named.SelfContained() {
		t.Fatal("named mod with ≥1 svg must be self-contained")
	}

	def := Mod{ID: "default", Root: dir, IsDefault: true}
	if def.SelfContained() {
		t.Fatal("default mod is overlay: never self-contained")
	}

	bare := Mod{ID: "lite", Root: t.TempDir(), IsDefault: false}
	if bare.SelfContained() {
		t.Fatal("named mod with no faces must inherit embedded (not self-contained)")
	}
}
