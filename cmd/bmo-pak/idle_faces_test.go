package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/carroarmato0/nextui-bmo/internal/assistant"
	"github.com/carroarmato0/nextui-bmo/internal/mod"
)

func TestModIdleFaces(t *testing.T) {
	// The default/overlay mod resolves every face → unfiltered (nil).
	if got := modIdleFaces(mod.Mod{IsDefault: true}); got != nil {
		t.Errorf("default mod should be unfiltered, got %v", got)
	}

	// A self-contained mod → exactly its on-disk faces (functional ones included).
	dir := t.TempDir()
	facesDir := filepath.Join(dir, "faces")
	if err := os.MkdirAll(facesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"neutral.svg", "angry.svg", "look_around.svg"} {
		if err := os.WriteFile(filepath.Join(facesDir, n), []byte("<svg/>"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// SelfContained: !IsDefault && FacesHasSVG — Open populates FS from Root.
	m := mod.Mod{ID: "evil", Root: dir}
	if err := m.Open(nil); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Close() }()
	set := modIdleFaces(m)

	for _, e := range []assistant.Expression{
		assistant.ExpressionNeutral, assistant.ExpressionAngry, assistant.ExpressionLookAround,
	} {
		if !set[e] {
			t.Errorf("expected %q to be available", e)
		}
	}
	if set[assistant.ExpressionSmile] {
		t.Error("smile is not shipped; it must not be in the available set")
	}
}
