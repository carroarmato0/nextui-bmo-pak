package main

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/carroarmato0/nextui-bmo/internal/mod"
)

type testLogger struct{}

func (testLogger) Infof(string, ...any)  {}
func (testLogger) Warnf(string, ...any)  {}
func (testLogger) Debugf(string, ...any) {}

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

func TestResolveWakeModelSanitizesModID(t *testing.T) {
	modFS := fstest.MapFS{"wakeword/wake.onnx": &fstest.MapFile{Data: []byte("x")}}
	tmp := t.TempDir()
	path, custom, err := resolveWakeModel(modFS, "../evil", "/pak/hey_bmo.onnx", tmp)
	if err != nil || !custom {
		t.Fatalf("got (%q,%v,%v)", path, custom, err)
	}
	if filepath.Dir(path) != tmp {
		t.Fatalf("extracted outside tmpDir: %q (tmp=%q)", path, tmp)
	}
	if filepath.Base(path) != "wake-evil.onnx" {
		t.Fatalf("base = %q, want wake-evil.onnx", filepath.Base(path))
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

func TestBuildWakeAssetsNoCustomUsesDefault(t *testing.T) {
	m := mod.Mod{ID: "plain", FS: fstest.MapFS{"faces/x.svg": &fstest.MapFile{Data: []byte("x")}}}
	assets, cleanup := buildWakeAssets(m, "/pak", "tg5040", t.TempDir(), testLogger{})
	defer cleanup()
	if assets.WakeModel != filepath.Join("/pak", "assets", "wakeword", "hey_bmo.onnx") {
		t.Fatalf("WakeModel = %q, want pak default", assets.WakeModel)
	}
	if assets.MelModel == "" || assets.EmbModel == "" || assets.ORTLib == "" {
		t.Fatal("base asset paths must be populated from pakDir")
	}
}

func TestBuildWakeAssetsInvalidCustomFallsBackAndCleansUp(t *testing.T) {
	// A custom model is present, but ORT cannot validate it here (no real
	// runtime / bogus pakDir lib), so buildWakeAssets must fall back to the
	// default AND remove the extracted temp file.
	m := mod.Mod{ID: "evil", FS: fstest.MapFS{"wakeword/wake.onnx": &fstest.MapFile{Data: []byte("not-a-real-onnx")}}}
	tmp := t.TempDir()
	assets, cleanup := buildWakeAssets(m, "/pak", "tg5040", tmp, testLogger{})
	defer cleanup()
	if assets.WakeModel != filepath.Join("/pak", "assets", "wakeword", "hey_bmo.onnx") {
		t.Fatalf("WakeModel = %q, want pak default after invalid custom", assets.WakeModel)
	}
	if _, err := os.Stat(filepath.Join(tmp, "wake-evil.onnx")); !os.IsNotExist(err) {
		t.Fatalf("temp extracted model should have been cleaned up, stat err = %v", err)
	}
}
