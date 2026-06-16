package face

import "testing"

func TestDefaultCoreAnimations(t *testing.T) {
	defs := DefaultAnimations()
	for _, name := range []string{
		ExprNeutral, ExprHappy, ExprSmile, ExprExcited,
		ExprContent, ExprConcerned, ExprSad, ExprAngry,
	} {
		d, ok := defs[name]
		if !ok {
			t.Fatalf("%s: default animation missing", name)
		}
		if d.Template == nil {
			t.Fatalf("%s: expected a template def", name)
		}
		if d.Template.Param != "m" || d.Template.Steps != 6 {
			t.Fatalf("%s: template=%+v", name, *d.Template)
		}
		if d.Driver.Kind != DriverAmplitude || d.Driver.Curve != curveSqrt {
			t.Fatalf("%s: driver=%+v", name, d.Driver)
		}
		if d.Driver.Idle != nil {
			t.Fatalf("%s: core defs must have NO idle (rest at silence)", name)
		}
	}
	if _, ok := defs[ExprSpeaking]; ok {
		t.Fatal("standalone speaking def should be retired")
	}
}

func TestDefaultCoreFramesRasterizeAndDiffer(t *testing.T) {
	lib := NewLibrary(t.TempDir())
	for _, name := range []string{ExprNeutral, ExprHappy, ExprSad} {
		frames, err := buildFrames(lib, DefaultAnimations()[name], 80, 60)
		if err != nil {
			t.Fatalf("%s: buildFrames: %v", name, err)
		}
		if len(frames) != 6 {
			t.Fatalf("%s: frames=%d want 6", name, len(frames))
		}
		if equalFrame(frames[0], frames[5]) {
			t.Fatalf("%s: rest and open frames identical", name)
		}
	}
}

func equalFrame(a, b []uint32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
