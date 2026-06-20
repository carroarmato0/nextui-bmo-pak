package main

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestResolveWakeModelExtractsCustom(t *testing.T) {
	want := []byte("fake-onnx-bytes")
	modFS := fstest.MapFS{"wakeword/wake.onnx": &fstest.MapFile{Data: want}}
	tmp := t.TempDir()

	path, custom, err := resolveWakeModel(modFS, "evil-bmo", "/pak/hey_bmo.onnx", tmp)
	if err != nil {
		t.Fatalf("resolveWakeModel: %v", err)
	}
	if !custom {
		t.Fatal("expected custom=true when wakeword/wake.onnx is present")
	}
	if path != filepath.Join(tmp, "wake-evil-bmo.onnx") {
		t.Fatalf("path = %q, want tmp/wake-evil-bmo.onnx", path)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read extracted: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("extracted bytes = %q, want %q", got, want)
	}
}

func TestResolveWakeModelFallsBackWhenAbsent(t *testing.T) {
	modFS := fstest.MapFS{"faces/neutral.svg": &fstest.MapFile{Data: []byte("x")}}
	path, custom, err := resolveWakeModel(modFS, "plain", "/pak/hey_bmo.onnx", t.TempDir())
	if err != nil {
		t.Fatalf("resolveWakeModel: %v", err)
	}
	if custom {
		t.Fatal("expected custom=false when no wakeword/wake.onnx")
	}
	if path != "/pak/hey_bmo.onnx" {
		t.Fatalf("path = %q, want default", path)
	}
}

func TestResolveWakeModelNilFS(t *testing.T) {
	path, custom, err := resolveWakeModel(nil, "x", "/pak/hey_bmo.onnx", t.TempDir())
	if err != nil || custom || path != "/pak/hey_bmo.onnx" {
		t.Fatalf("nil FS: got (%q,%v,%v), want default", path, custom, err)
	}
}
