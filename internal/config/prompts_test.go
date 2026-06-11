package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsurePromptFileCreatesMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "persona.txt")
	got, err := EnsurePromptFile(path, "the default")
	if err != nil {
		t.Fatalf("EnsurePromptFile() error = %v", err)
	}
	if got != "the default" {
		t.Fatalf("content = %q, want default", got)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	if string(data) != "the default\n" {
		t.Fatalf("file content = %q, want default with trailing newline", string(data))
	}
}

func TestEnsurePromptFileFillsBlank(t *testing.T) {
	path := filepath.Join(t.TempDir(), "voice.txt")
	if err := os.WriteFile(path, []byte("  \n\t\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := EnsurePromptFile(path, "the default")
	if err != nil {
		t.Fatalf("EnsurePromptFile() error = %v", err)
	}
	if got != "the default" {
		t.Fatalf("content = %q, want default for blank file", got)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "the default\n" {
		t.Fatalf("blank file not filled with default: %q", string(data))
	}
}

func TestEnsurePromptFileKeepsContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "persona.txt")
	custom := "My custom persona.\n\nWith multiple lines.\n"
	if err := os.WriteFile(path, []byte(custom), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := EnsurePromptFile(path, "the default")
	if err != nil {
		t.Fatalf("EnsurePromptFile() error = %v", err)
	}
	if got != "My custom persona.\n\nWith multiple lines." {
		t.Fatalf("content = %q, want trimmed custom content", got)
	}
	data, _ := os.ReadFile(path)
	if string(data) != custom {
		t.Fatalf("existing file was modified: %q", string(data))
	}
}

func TestPromptPaths(t *testing.T) {
	if got := PersonaPath("/home/bmo"); got != "/home/bmo/persona.txt" {
		t.Fatalf("PersonaPath = %q", got)
	}
	if got := VoicePath("/home/bmo"); got != "/home/bmo/voice.txt" {
		t.Fatalf("VoicePath = %q", got)
	}
}
