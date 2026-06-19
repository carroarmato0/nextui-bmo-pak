package examplemods

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/carroarmato0/nextui-bmo/internal/config"
	"github.com/carroarmato0/nextui-bmo/internal/face"
	modpkg "github.com/carroarmato0/nextui-bmo/internal/mod"
)

// modRoot points at the Evil BMO example mod, relative to this package's
// directory. go test runs with the package dir as the working directory, so the
// mod is two levels up under examples/mods. Keeping the test here (rather than
// inside the mod dir) lets examples/mods/evil-bmo stay a data-only showcase.
func modRoot(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "examples", "mods", "evil-bmo")
}

// TestDeviceValidation mirrors exactly what the app runs when it loads a mod:
// config.CheckOverrides validates that persona/voice/quotes are non-blank and
// that every faces/*.svg is valid XML on the RAW bytes — before any template
// execution. This is stricter than TestFacesRender (which RenderRest's first),
// so it catches template syntax that is only valid after rendering, e.g. a
// quote character inside an attribute value (cx="{{printf "%.1f" $x}}"). The
// device rejects such a face as "not valid XML" and falls back to neutral.
func TestDeviceValidation(t *testing.T) {
	if errs := config.CheckOverrides(os.DirFS(modRoot(t))); len(errs) != 0 {
		for _, e := range errs {
			t.Errorf("CheckOverrides (device mod-load validation): %v", e)
		}
	}
}

func TestManifest(t *testing.T) {
	m := modpkg.LoadManifest(os.DirFS(modRoot(t)))
	if got := m.EffectiveAPIVersion(); got != modpkg.CurrentAPIVersion {
		t.Errorf("apiVersion = %d, want %d", got, modpkg.CurrentAPIVersion)
	}
	if m.Name != "Evil BMO" {
		t.Errorf("name = %q, want %q", m.Name, "Evil BMO")
	}
	if strings.TrimSpace(m.Description) == "" {
		t.Error("description is blank")
	}
	if strings.TrimSpace(m.Version) == "" {
		t.Error("version is blank")
	}
}

func TestEmotions(t *testing.T) {
	m := modpkg.LoadManifest(os.DirFS(modRoot(t)))
	for _, key := range []string{"neutral", "laugh", "angry", "skeptical", "unamused", "smug"} {
		if _, ok := m.Emotions[key]; !ok {
			t.Errorf("emotions missing key %q", key)
		}
	}
	// Emotions that ship a dedicated face must have a matching SVG, so a
	// typo'd key (e.g. "skeptcal") is caught here rather than silently
	// folding to neutral on-device. The snob aliases (smug/mocking/gloating)
	// intentionally have no face and are excluded.
	for _, key := range []string{"neutral", "laugh", "angry", "skeptical", "unamused"} {
		facePath := filepath.Join(modRoot(t), "faces", key+".svg")
		if _, err := os.Stat(facePath); err != nil {
			t.Errorf("emotion %q has no matching faces/%s.svg", key, key)
		}
	}
}

// TestAudioClips checks the mod ships every spoken system clip as a non-empty
// .pcm. The app folds a missing clip to the embedded default (the cheerful BMO
// voice), so a missing file would silently break character rather than error.
func TestAudioClips(t *testing.T) {
	for _, name := range []string{"hello", "goodbye", "error", "timeout", "mod_error", "sleep", "wake"} {
		p := filepath.Join(modRoot(t), "audio", name+".pcm")
		info, err := os.Stat(p)
		if err != nil {
			t.Errorf("missing audio clip %s.pcm: %v", name, err)
			continue
		}
		if info.Size() < 1024 {
			t.Errorf("audio clip %s.pcm is suspiciously small (%d bytes)", name, info.Size())
		}
	}
}

func TestPrompts(t *testing.T) {
	root := modRoot(t)
	for _, name := range []string{"persona.txt", "voice.txt", "quotes.txt"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if strings.TrimSpace(string(data)) == "" {
			t.Errorf("%s is blank", name)
		}
	}
	persona, _ := os.ReadFile(filepath.Join(root, "persona.txt"))
	if len(persona) > 1000 {
		t.Errorf("persona.txt is %d bytes, want <= 1000", len(persona))
	}
	quotes, _ := os.ReadFile(filepath.Join(root, "quotes.txt"))
	n := 0
	for _, line := range strings.Split(string(quotes), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			n++
		}
	}
	if n < 20 {
		t.Errorf("quotes.txt has %d usable lines, want >= 20", n)
	}
}

func TestSelfContained(t *testing.T) {
	root := modRoot(t)
	m := modpkg.Mod{ID: "evil-bmo", Root: root, Manifest: modpkg.LoadManifest(os.DirFS(root))}
	if err := m.Open(nil); err != nil {
		t.Fatalf("open mod: %v", err)
	}
	defer func() { _ = m.Close() }()
	if !m.FacesHasSVG() {
		t.Fatal("FacesHasSVG() = false, want true (faces/ must hold >=1 .svg)")
	}
	if !m.SelfContained() {
		t.Error("SelfContained() = false, want true")
	}
}

// renderFace runs a face SVG through the exact device path: RenderRest
// (execute the template at rest) then Rasterize. It also asserts the rested
// SVG is well-formed XML, catching unclosed tags and broken templates.
func renderFace(t *testing.T, path string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	svg := face.RenderRest(raw)
	dec := xml.NewDecoder(bytes.NewReader(svg))
	for {
		_, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("%s: not well-formed XML after RenderRest: %v", filepath.Base(path), err)
		}
	}
	if _, err := face.Rasterize(svg, 280, 210); err != nil {
		t.Fatalf("%s: rasterize failed: %v", filepath.Base(path), err)
	}
}

func TestFacesRender(t *testing.T) {
	dir := filepath.Join(modRoot(t), "faces")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read faces dir: %v", err)
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".svg") {
			continue
		}
		count++
		name := e.Name()
		t.Run(name, func(t *testing.T) { renderFace(t, filepath.Join(dir, name)) })
	}
	if count == 0 {
		t.Fatal("no .svg faces found")
	}
}

func TestAnimations(t *testing.T) {
	root := modRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "mod.json"))
	if err != nil {
		t.Fatalf("read mod.json: %v", err)
	}
	var manifest struct {
		Animations map[string]json.RawMessage `json:"animations"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("unmarshal mod.json: %v", err)
	}
	defs, errs := face.ParseAnimations(manifest.Animations)
	if len(errs) != 0 {
		t.Fatalf("ParseAnimations errors: %v", errs)
	}
	for _, key := range []string{"neutral", "laugh", "angry", "speaking", "look_around"} {
		def, ok := defs[key]
		if !ok {
			t.Errorf("animation %q missing", key)
			continue
		}
		if def.Template == nil {
			t.Errorf("animation %q is not template-based", key)
			continue
		}
		facePath := filepath.Join(root, "faces", def.Template.File+".svg")
		if _, err := os.Stat(facePath); err != nil {
			t.Errorf("animation %q references missing face %s.svg", key, def.Template.File)
		}
	}
}

// zipExampleMod packages examples/mods/evil-bmo into <tmp>/evil-bmo.zip with a
// top-level evil-bmo/ folder, exactly like scripts/release.sh, and returns the
// archive path.
func zipExampleMod(t *testing.T) string {
	t.Helper()
	src := modRoot(t)
	dst := filepath.Join(t.TempDir(), "evil-bmo.zip")
	f, err := os.Create(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	err = filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		w, err := zw.Create(filepath.ToSlash(filepath.Join("evil-bmo", rel)))
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	})
	if err != nil {
		t.Fatalf("walk/zip: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return dst
}

func TestZippedExampleModValidates(t *testing.T) {
	m := modpkg.Mod{ID: "evil-bmo", Root: zipExampleMod(t)}
	if err := m.Open(nil); err != nil {
		t.Fatalf("Open zip: %v", err)
	}
	defer m.Close()

	if errs := config.CheckOverrides(m.FS); len(errs) != 0 {
		t.Errorf("CheckOverrides on zip: %v", errs)
	}
	if got := modpkg.LoadManifest(m.FS).Name; got != "Evil BMO" {
		t.Errorf("zip manifest Name = %q, want %q", got, "Evil BMO")
	}
	if !m.FacesHasSVG() || !m.SelfContained() {
		t.Error("zipped evil-bmo should be self-contained")
	}
}
