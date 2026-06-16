package face

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

// newExpressions are the still-static faces from the Figma "BMO Face Templates"
// set, in the same stable order as CanonicalNames. Each shipped asset must stay
// byte-identical to the artifact approved in the browser preview. The core-set
// emotions (happy, content, angry, sad, excited) are now param-driven templates
// rather than frozen byte-art, so they are guarded by the animation tests
// instead of this manifest.
var newExpressions = []string{
	ExprSurprised, ExprLove, ExprShy, ExprCrying, ExprTeary, ExprGloomy,
	ExprDizzy, ExprUnamused, ExprAnnoyed, ExprSkeptical, ExprPlayful,
	ExprKiss, ExprGrimace, ExprShout, ExprDead, ExprGlitch, ExprDismayed,
	ExprAdoring, ExprSparkle,
}

// TestNewExpressionFidelity guards that every shipped face is byte-identical to
// the frozen, browser-approved baseline. Byte-identity is used (instead of a
// golden render hash) so the check is deterministic across machines, Go, and
// oksvg versions; because the browser preview rendered these exact SVG bytes,
// byte-fidelity to them is fidelity to what was approved.
func TestNewExpressionFidelity(t *testing.T) {
	raw, err := os.ReadFile("testdata/approved_expressions.json")
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var approved map[string]string
	if err := json.Unmarshal(raw, &approved); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	if len(approved) != len(newExpressions) {
		t.Fatalf("manifest has %d entries, want %d", len(approved), len(newExpressions))
	}
	for _, name := range newExpressions {
		data, ok := defaultBytes(name)
		if !ok {
			t.Errorf("%s: no embedded SVG", name)
			continue
		}
		want, ok := approved[name]
		if !ok {
			t.Errorf("%s: missing from approved manifest", name)
			continue
		}
		sum := sha256.Sum256(data)
		got := hex.EncodeToString(sum[:])
		if got != want {
			t.Errorf("%s: sha256 %s != approved %s — asset no longer matches the browser-approved preview", name, got, want)
		}
	}
}

// TestNewExpressionsRasterize confirms every new asset renders non-blank through
// the real device path (oksvg) at both device resolutions. Rasterize returns an
// error on blank output, so a successful call proves visible pixels.
func TestNewExpressionsRasterize(t *testing.T) {
	for _, size := range [][2]int{{1024, 768}, {1280, 720}} {
		w, h := size[0], size[1]
		for _, name := range newExpressions {
			data, ok := defaultBytes(name)
			if !ok {
				t.Fatalf("no embedded SVG for %q", name)
			}
			if _, err := Rasterize(data, w, h); err != nil {
				t.Errorf("rasterize %s at %dx%d: %v", name, w, h, err)
			}
		}
	}
}
