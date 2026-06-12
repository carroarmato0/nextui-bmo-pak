package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPromptFileAbsent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "persona.txt")
	if got := LoadPromptFile(path, "default content"); got != "default content" {
		t.Fatalf("absent file: got %q, want default", got)
	}
}

func TestLoadPromptFileBlank(t *testing.T) {
	path := filepath.Join(t.TempDir(), "persona.txt")
	if err := os.WriteFile(path, []byte("  \n\t"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := LoadPromptFile(path, "default content"); got != "default content" {
		t.Fatalf("blank file: got %q, want default", got)
	}
}

func TestLoadPromptFileOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "persona.txt")
	if err := os.WriteFile(path, []byte(" custom persona \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := LoadPromptFile(path, "default content"); got != "custom persona" {
		t.Fatalf("override: got %q, want trimmed custom content", got)
	}
}

func TestRemoveOverrides(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "persona.txt")
	b := filepath.Join(dir, "voice.txt")
	missing := filepath.Join(dir, "quotes.txt")
	if err := os.WriteFile(a, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("y"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RemoveOverrides(a, b, missing); err != nil {
		t.Fatalf("RemoveOverrides with missing file: %v", err)
	}
	for _, p := range []string{a, b} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("file %s should have been removed", p)
		}
	}
}

func TestPromptPaths(t *testing.T) {
	if got := PersonaPath("/home/bmo"); got != "/home/bmo/persona.txt" {
		t.Fatalf("PersonaPath = %q", got)
	}
	if got := VoicePath("/home/bmo"); got != "/home/bmo/voice.txt" {
		t.Fatalf("VoicePath = %q", got)
	}
	if got := FacesDir("/home/bmo"); got != "/home/bmo/faces" {
		t.Fatalf("FacesDir = %q", got)
	}
}
