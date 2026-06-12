package face

import (
	"strings"
	"testing"
)

func TestSpeakLevelsMonotonic(t *testing.T) {
	data, ok := defaultBytes(ExprSpeaking)
	if !ok {
		t.Fatal("embedded speaking.svg missing")
	}
	if !IsSpeakTemplate(data) {
		t.Fatal("embedded speaking.svg must contain template markers")
	}
	w, h := 560, 420
	var prev int
	for lvl := 0; lvl < speakLevels; lvl++ {
		svg, err := renderSpeakSVG(data, float64(lvl)/float64(speakLevels-1))
		if err != nil {
			t.Fatalf("render level %d: %v", lvl, err)
		}
		buf, err := Rasterize(svg, w, h)
		if err != nil {
			t.Fatalf("rasterize level %d: %v", lvl, err)
		}
		// Count dark pixels in mouth band (viewBox y 96..150, x 75..171)
		bx0, by0, bx1, by1 := speakBand(w, h)
		count := 0
		for row := by0; row < by1; row++ {
			for col := bx0; col < bx1; col++ {
				px := buf[row*w+col]
				r, g, b := uint8(px>>16), uint8(px>>8), uint8(px)
				if int(r)+int(g)+int(b) < 200 {
					count++
				}
			}
		}
		if lvl > 0 && count < prev {
			t.Errorf("level %d: dark pixel count %d < level %d count %d (not monotonically increasing)", lvl, count, lvl-1, prev)
		}
		prev = count
	}
}

func TestSpeakFullOpen(t *testing.T) {
	data, _ := defaultBytes(ExprSpeaking)
	svg, err := renderSpeakSVG(data, 1.0)
	if err != nil {
		t.Fatal(err)
	}
	buf, err := Rasterize(svg, 1024, 768)
	if err != nil {
		t.Fatal(err)
	}
	// Fully open: pill eyes, teeth top of mouth, interior below
	assertColor(t, buf, 1024, 768, 62, 78, 0x1a, 0x1a, 0x1a, "left pill eye")
	assertColor(t, buf, 1024, 768, 123, 109, 0xe4, 0xe4, 0xe4, "teeth")
	assertColor(t, buf, 1024, 768, 123, 128, 0x1a, 0x78, 0x48, "interior")
}

func TestIsSpeakTemplate(t *testing.T) {
	if IsSpeakTemplate([]byte(`<svg viewBox="0 0 280 210"></svg>`)) {
		t.Fatal("plain SVG must not be detected as template")
	}
	if !IsSpeakTemplate([]byte(`<rect height="{{.MouthH}}"/>`)) {
		t.Fatal("SVG with {{ must be detected as template")
	}
	if !strings.Contains(string(mustDefault(ExprSpeaking)), "{{") {
		t.Fatal("embedded speaking.svg must contain template markers")
	}
}

func mustDefault(name string) []byte {
	data, ok := defaultBytes(name)
	if !ok {
		panic("missing embedded asset: " + name)
	}
	return data
}
