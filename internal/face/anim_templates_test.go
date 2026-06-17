package face

import "testing"

// talkingEmotions are every emotion whose mouth is driven by voice amplitude:
// they render their own resting mouth at silence and open the shared
// teeth/tongue talkmouth while speaking.
var talkingEmotions = []string{
	ExprNeutral, ExprHappy, ExprSmile, ExprExcited,
	ExprContent, ExprConcerned, ExprSad, ExprAngry,
	ExprPlayful, ExprAdoring, ExprSparkle, ExprLove, ExprShy,
	ExprSurprised, ExprGloomy, ExprAnnoyed, ExprSkeptical,
	ExprDismayed, ExprUnamused,
}

func TestCoreTemplatesRenderRestAndOpen(t *testing.T) {
	lib := NewLibrary(t.TempDir()) // embedded assets only
	for _, name := range talkingEmotions {
		data, ok := lib.rawBytes(name)
		if !ok {
			t.Fatalf("%s: no embedded bytes", name)
		}
		// Rest (no data) must rasterize.
		rest, err := Rasterize(renderRestSVG(data), 80, 60)
		if err != nil {
			t.Fatalf("%s: rest rasterize: %v", name, err)
		}
		// Full open (m=1) must rasterize and differ from rest.
		openSVG, err := renderAnimTemplate(data, "m", 1)
		if err != nil {
			t.Fatalf("%s: render m=1: %v", name, err)
		}
		open, err := Rasterize(openSVG, 80, 60)
		if err != nil {
			t.Fatalf("%s: open rasterize: %v", name, err)
		}
		if equalFrame(rest, open) {
			t.Fatalf("%s: rest and open frames identical (mouth not animating)", name)
		}
	}
}

func TestEmotionTalkingMouthOpensWithTeeth(t *testing.T) {
	lib := NewLibrary(t.TempDir()) // embedded assets only

	const (
		teeth  = 0xe4e4e4 // white teeth band
		tongue = 0x1a7848 // dark green mouth interior
	)
	// hasInMouthBand scans only the rows below the eyes (the shared mouth lives at
	// y≈106–142 in the 280×210 viewBox), so white eye highlights — which some
	// emotions draw in #e4e4e4 — never masquerade as teeth.
	const w, h = 280, 210
	hasInMouthBand := func(buf []uint32, rgb uint32) bool {
		for y := 100; y < 150 && y < h; y++ {
			for x := 100; x < 185 && x < w; x++ {
				if buf[y*w+x]&0x00ffffff == rgb {
					return true
				}
			}
		}
		return false
	}

	for _, name := range talkingEmotions {
		t.Run(name, func(t *testing.T) {
			data, ok := lib.rawBytes(name)
			if !ok {
				t.Fatalf("%s: no embedded bytes", name)
			}
			rest, err := Rasterize(renderRestSVG(data), w, h)
			if err != nil {
				t.Fatalf("%s: rest rasterize: %v", name, err)
			}
			openSVG, err := renderAnimTemplate(data, "m", 1)
			if err != nil {
				t.Fatalf("%s: render m=1: %v", name, err)
			}
			open, err := Rasterize(openSVG, w, h)
			if err != nil {
				t.Fatalf("%s: open rasterize: %v", name, err)
			}
			if !hasInMouthBand(open, teeth) {
				t.Errorf("%s: open frame missing teeth (#e4e4e4) in mouth band", name)
			}
			if !hasInMouthBand(open, tongue) {
				t.Errorf("%s: open frame missing tongue interior (#1a7848) in mouth band", name)
			}
			// Every talking emotion rests on its own line/curve mouth: no teeth band
			// at silence, so it opens the shared mouth in place instead of snapping.
			if hasInMouthBand(rest, teeth) {
				t.Errorf("%s: teeth present at rest — natural mouth should show at silence", name)
			}
		})
	}
}
