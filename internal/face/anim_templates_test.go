package face

import "testing"

func TestCoreTemplatesRenderRestAndOpen(t *testing.T) {
	lib := NewLibrary(t.TempDir()) // embedded assets only
	for _, name := range []string{
		ExprNeutral, ExprHappy, ExprSmile, ExprExcited,
		ExprContent, ExprConcerned, ExprSad, ExprAngry,
	} {
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
	has := func(buf []uint32, rgb uint32) bool {
		for _, px := range buf {
			if px&0x00ffffff == rgb {
				return true
			}
		}
		return false
	}

	// Emotions whose natural resting mouth is a thin line/curve (no teeth at
	// rest); they open the shared teeth/tongue mouth only while speaking.
	// Excited and smile rest as a closed grin at the talking-mouth width so
	// they open vertically in place instead of snapping from a wider open grin.
	lineMouth := map[string]bool{
		ExprNeutral: true, ExprHappy: true, ExprSad: true,
		ExprContent: true, ExprConcerned: true, ExprAngry: true,
		ExprExcited: true, ExprSmile: true,
	}

	for _, name := range []string{
		ExprNeutral, ExprHappy, ExprSmile, ExprExcited,
		ExprContent, ExprConcerned, ExprSad, ExprAngry,
	} {
		t.Run(name, func(t *testing.T) {
			data, ok := lib.rawBytes(name)
			if !ok {
				t.Fatalf("%s: no embedded bytes", name)
			}
			rest, err := Rasterize(renderRestSVG(data), 280, 210)
			if err != nil {
				t.Fatalf("%s: rest rasterize: %v", name, err)
			}
			openSVG, err := renderAnimTemplate(data, "m", 1)
			if err != nil {
				t.Fatalf("%s: render m=1: %v", name, err)
			}
			open, err := Rasterize(openSVG, 280, 210)
			if err != nil {
				t.Fatalf("%s: open rasterize: %v", name, err)
			}
			if !has(open, teeth) {
				t.Errorf("%s: open frame missing teeth (#e4e4e4)", name)
			}
			if !has(open, tongue) {
				t.Errorf("%s: open frame missing tongue interior (#1a7848)", name)
			}
			if lineMouth[name] && has(rest, teeth) {
				t.Errorf("%s: teeth present at rest — natural mouth should show at silence", name)
			}
		})
	}
}
