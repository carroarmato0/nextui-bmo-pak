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
