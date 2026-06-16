package face

import "testing"

func TestDefaultSpeakingAnimation(t *testing.T) {
	defs := DefaultAnimations()
	sp, ok := defs[ExprSpeaking]
	if !ok {
		t.Fatal("speaking default missing")
	}
	if sp.Steps() != 6 {
		t.Fatalf("speaking steps=%d want 6", sp.Steps())
	}
	if sp.Driver.Kind != DriverAmplitude || sp.Driver.Curve != "sqrt" || sp.Driver.Idle == nil {
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
	// Every frame is a full buffer (Rasterize already rejects blank frames).
	for i, f := range frames {
		if len(f) != 80*60 {
			t.Fatalf("frame %d size=%d want %d", i, len(f), 80*60)
		}
	}
	// Closed (step 0) and fully-open (step 5) mouths must differ.
	if equalFrame(frames[0], frames[5]) {
		t.Fatal("closed and open speaking frames are identical")
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
