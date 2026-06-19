package face

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

const tinyRedSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><rect x="0" y="0" width="10" height="10" fill="#ff0000"/></svg>`

func TestAnimFuncsArithmetic(t *testing.T) {
	in := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10">` +
		`{{$m := or .m 0.0}}<rect y="{{add 125.0 (mul 22.0 $m)}}"/></svg>`)
	rest := renderRestSVG(in)
	if !bytes.Contains(rest, []byte(`y="125"`)) {
		t.Fatalf("rest arithmetic wrong: %s", rest)
	}
	open, err := renderAnimTemplate(in, "m", 1)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !bytes.Contains(open, []byte(`y="147"`)) {
		t.Fatalf("open arithmetic wrong: %s", open)
	}
}

func writeSVG(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name+".svg"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBuildFramesList(t *testing.T) {
	dir := t.TempDir()
	writeSVG(t, dir, "f0", tinyRedSVG)
	writeSVG(t, dir, "f1", tinyRedSVG)
	lib := NewLibraryMode(os.DirFS(dir), true) // self-contained: only on-disk frames
	def := AnimationDef{Frames: []string{"f0", "f1"}, Driver: Driver{Kind: DriverAmplitude, Curve: "linear"}}
	frames, err := buildFrames(lib, def, 4, 4)
	if err != nil {
		t.Fatalf("buildFrames: %v", err)
	}
	if len(frames) != 2 || len(frames[0]) != 16 {
		t.Fatalf("frames=%d size=%d", len(frames), len(frames[0]))
	}
}

func TestBuildFramesTemplate(t *testing.T) {
	dir := t.TempDir()
	// width grows with V so steps differ
	tmpl := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><rect x="0" y="0" width="{{.V}}" height="10" fill="#0000ff"/></svg>`
	writeSVG(t, dir, "bar", tmpl)
	lib := NewLibraryMode(os.DirFS(dir), true)
	def := AnimationDef{
		Template: &TemplateSource{File: "bar", Param: "V", From: 1, To: 10, Steps: 3},
		Driver:   Driver{Kind: DriverTime, FPS: 4, Mode: "loop"},
	}
	frames, err := buildFrames(lib, def, 8, 8)
	if err != nil {
		t.Fatalf("buildFrames: %v", err)
	}
	if len(frames) != 3 {
		t.Fatalf("frames=%d want 3", len(frames))
	}
}

func TestRawBytesPrefersOverrideThenEmbedded(t *testing.T) {
	dir := t.TempDir()
	writeSVG(t, dir, "speaking_0", tinyRedSVG)
	lib := NewLibrary(os.DirFS(dir)) // overlay: embedded fallback enabled
	data, ok := lib.rawBytes("speaking_0")
	if !ok || len(data) == 0 {
		t.Fatal("override speaking_0 not found")
	}
	// neutral is embedded; rawBytes finds it by literal name with no override.
	if d, ok := NewLibrary(os.DirFS(t.TempDir())).rawBytes("neutral"); !ok || len(d) == 0 {
		t.Fatal("embedded neutral not found by rawBytes")
	}
}

func TestBuildFramesMissingFrameErrors(t *testing.T) {
	lib := NewLibraryMode(os.DirFS(t.TempDir()), true)
	def := AnimationDef{Frames: []string{"nope"}, Driver: Driver{Kind: DriverAmplitude}}
	if _, err := buildFrames(lib, def, 4, 4); err == nil {
		t.Fatal("expected error for missing frame")
	}
}

func TestRenderRestSVGExecutesTemplate(t *testing.T) {
	in := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">` +
		`{{$m := or .m 0.0}}<rect x="0" y="{{$m}}" width="10" height="10"/></svg>`)
	out := renderRestSVG(in)
	if bytes.Contains(out, []byte("{{")) {
		t.Fatalf("template syntax left in output: %s", out)
	}
	// At rest, m=0 → the y attribute renders as "0".
	if !bytes.Contains(out, []byte(`y="0"`)) {
		t.Fatalf("rest value not 0: %s", out)
	}
	if _, err := Rasterize(out, 80, 60); err != nil {
		t.Fatalf("rest SVG must rasterize: %v", err)
	}
}

func TestRenderRestSVGPassesThroughPlain(t *testing.T) {
	in := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210"><rect/></svg>`)
	out := renderRestSVG(in)
	if !bytes.Equal(in, out) {
		t.Fatalf("plain SVG should pass through unchanged")
	}
}
