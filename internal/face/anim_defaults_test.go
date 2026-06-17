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
}

func TestDefaultSpeakingAnimation(t *testing.T) {
	defs := DefaultAnimations()
	sp, ok := defs[ExprSpeaking]
	if !ok {
		t.Fatal("speaking default missing")
	}
	if sp.Template != nil {
		t.Fatal("speaking should be a discrete-frame def, not a template")
	}
	if sp.Steps() != 6 {
		t.Fatalf("speaking steps=%d want 6", sp.Steps())
	}
	if sp.Driver.Kind != DriverAmplitude || sp.Driver.Curve != curveSqrt || sp.Driver.Idle == nil {
		t.Fatalf("driver=%+v", sp.Driver)
	}
}

func TestDefaultSpeakingFramesRasterizeAndDiffer(t *testing.T) {
	lib := NewLibrary(t.TempDir()) // overlay: embedded assets only
	frames, err := buildFrames(lib, DefaultAnimations()[ExprSpeaking], 80, 60)
	if err != nil {
		t.Fatalf("buildFrames: %v", err)
	}
	if len(frames) != 6 {
		t.Fatalf("frames=%d want 6", len(frames))
	}
	if equalFrame(frames[0], frames[5]) {
		t.Fatal("closed and open speaking frames are identical")
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

func TestDefaultWhistleAnimation(t *testing.T) {
	def, ok := DefaultAnimations()[ExprWhistle]
	if !ok {
		t.Fatal("whistle default missing")
	}
	if def.Template == nil || def.Template.Param != "t" || def.Template.Steps != 6 {
		t.Fatalf("template=%+v", def.Template)
	}
	if def.Driver.Kind != DriverTime || def.Driver.Mode != modeLoop || def.Driver.FPS != 4 {
		t.Fatalf("driver=%+v", def.Driver)
	}
	lib := NewLibrary(t.TempDir())
	frames, err := buildFrames(lib, def, 80, 60)
	if err != nil {
		t.Fatalf("buildFrames: %v", err)
	}
	if len(frames) != 6 {
		t.Fatalf("frames=%d want 6", len(frames))
	}
	if equalFrame(frames[0], frames[5]) {
		t.Fatal("note-low and note-high whistle frames are identical")
	}
}

func TestDefaultLookAroundAnimation(t *testing.T) {
	def, ok := DefaultAnimations()[ExprLookAround]
	if !ok {
		t.Fatal("look_around default missing")
	}
	if def.Template == nil || def.Template.Param != "x" || def.Template.Steps != 5 {
		t.Fatalf("template=%+v", def.Template)
	}
	if def.Driver.Kind != DriverTime || def.Driver.Mode != modePingpong || def.Driver.FPS != 3 {
		t.Fatalf("driver=%+v", def.Driver)
	}
	lib := NewLibrary(t.TempDir())
	frames, err := buildFrames(lib, def, 80, 60)
	if err != nil {
		t.Fatalf("buildFrames: %v", err)
	}
	if len(frames) != 5 {
		t.Fatalf("frames=%d want 5", len(frames))
	}
	if equalFrame(frames[0], frames[4]) {
		t.Fatal("eyes-left and eyes-right frames are identical")
	}
}
