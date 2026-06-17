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

// TestDefaultIdleAnimations covers the time-driven idle faces (look_around,
// whistle) that play during silence: each must be a template animation on a
// DriverTime, build the expected number of frames, and visibly differ between
// its first and last frame (the eye scan / the rising note actually move).
func TestDefaultIdleAnimations(t *testing.T) {
	cases := []struct {
		name  string
		param string
		steps int
		mode  string
		fps   float64
	}{
		{ExprLookAround, "x", 5, modePingpong, 3},
		{ExprWhistle, "t", 6, modeLoop, 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			def, ok := DefaultAnimations()[tc.name]
			if !ok {
				t.Fatalf("%s default missing", tc.name)
			}
			if def.Template == nil || def.Template.Param != tc.param || def.Template.Steps != tc.steps {
				t.Fatalf("template=%+v", def.Template)
			}
			if def.Driver.Kind != DriverTime || def.Driver.Mode != tc.mode || def.Driver.FPS != tc.fps {
				t.Fatalf("driver=%+v", def.Driver)
			}
			lib := NewLibrary(t.TempDir())
			frames, err := buildFrames(lib, def, 80, 60)
			if err != nil {
				t.Fatalf("buildFrames: %v", err)
			}
			if len(frames) != tc.steps {
				t.Fatalf("frames=%d want %d", len(frames), tc.steps)
			}
			if equalFrame(frames[0], frames[len(frames)-1]) {
				t.Fatal("first and last idle frames are identical (no motion)")
			}
		})
	}
}
