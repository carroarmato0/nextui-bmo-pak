package renderer

import (
	"testing"

	"github.com/carroarmato0/nextui-bmo/internal/face"
)

// framesStub serves built frames per expression; expressions absent from the
// map report "not ready" (nil,false), like an animation still building.
type framesStub struct {
	frames map[string][]uint32
}

func (framesStub) IsTimeDriven(string) bool { return false }
func (framesStub) FrameStep(string, int, int, float64, float64, float32) (int, bool) {
	return 0, false
}
func (s framesStub) AnimFrame(expr string, w, h int, clock, epoch float64, signal float32) ([]uint32, bool) {
	f, ok := s.frames[expr]
	return f, ok
}

func TestBlitFaceSpeakingFallsBackToSpeakingFaceWhenEmotionNotReady(t *testing.T) {
	speakingFrame := []uint32{11, 22, 33}
	r := &Renderer{W: 1, H: 3, pixels: make([]uint32, 3)}
	// "concerned" is absent (still building); only the pinned speaking face is ready.
	r.anims = framesStub{frames: map[string][]uint32{face.ExprSpeaking: speakingFrame}}

	if !r.blitFace("concerned", FrameState{Speaking: true}, 0, 0) {
		t.Fatal("blitFace should succeed via the speaking-face fallback")
	}
	for i, want := range speakingFrame {
		if r.pixels[i] != want {
			t.Fatalf("pixels = %v, want the moving speaking frame %v", r.pixels, speakingFrame)
		}
	}
}

func TestBlitFaceSpeakingPrefersEmotionWhenReady(t *testing.T) {
	concernedFrame := []uint32{7, 8, 9}
	speakingFrame := []uint32{1, 2, 3}
	r := &Renderer{W: 1, H: 3, pixels: make([]uint32, 3)}
	r.anims = framesStub{frames: map[string][]uint32{
		"concerned":       concernedFrame,
		face.ExprSpeaking: speakingFrame,
	}}

	if !r.blitFace("concerned", FrameState{Speaking: true}, 0, 0) {
		t.Fatal("blitFace should succeed with the emotion's own frames")
	}
	for i, want := range concernedFrame {
		if r.pixels[i] != want {
			t.Fatalf("pixels = %v, want the emotion frame %v (not the fallback)", r.pixels, concernedFrame)
		}
	}
}

func TestLayoutForScalesAcrossScreens(t *testing.T) {
	compact := LayoutFor(640, 480)
	wide := LayoutFor(1280, 720)

	if compact.W != 640 || compact.H != 480 {
		t.Fatalf("unexpected compact size: %+v", compact)
	}
	if wide.W != 1280 || wide.H != 720 {
		t.Fatalf("unexpected wide size: %+v", wide)
	}
	if compact.EyeW <= 0 || compact.EyeH <= 0 || compact.MouthW <= 0 {
		t.Fatalf("compact layout has invalid geometry: %+v", compact)
	}
	if wide.EyeW <= compact.EyeW {
		t.Fatalf("expected wider screen to allocate wider eyes: compact=%d wide=%d", compact.EyeW, wide.EyeW)
	}
	if wide.MouthW <= compact.MouthW {
		t.Fatalf("expected wider screen to allocate wider mouth: compact=%d wide=%d", compact.MouthW, wide.MouthW)
	}
}

func TestOverlayWindowClampsAndKeepsFocusVisible(t *testing.T) {
	// 10 visible rows, window of 4. Focus near the end must scroll.
	off := overlayWindow(10, 4, 9)
	if off < 0 || off > 6 {
		t.Fatalf("offset %d out of clamp range [0,6]", off)
	}
	if !(9 >= off && 9 < off+4) {
		t.Fatalf("focus 9 not within window [%d,%d)", off, off+4)
	}

	if off := overlayWindow(10, 4, 0); off != 0 {
		t.Fatalf("offset for focus 0 = %d, want 0", off)
	}

	if off := overlayWindow(3, 4, 2); off != 0 {
		t.Fatalf("offset when all fit = %d, want 0", off)
	}

	off = overlayWindow(10, 4, 8)
	if off+4 > 10 {
		t.Fatalf("window [%d,%d) overruns 10 rows", off, off+4)
	}
}

func TestOverlayWindowDegenerate(t *testing.T) {
	if off := overlayWindow(0, 4, 0); off != 0 {
		t.Fatalf("empty content offset = %d, want 0", off)
	}
	if off := overlayWindow(5, 0, 3); off != 0 {
		t.Fatalf("zero window offset = %d, want 0", off)
	}
}

func TestFramebufferEqual(t *testing.T) {
	a := []uint32{1, 2, 3}
	if !framebufferEqual(a, []uint32{1, 2, 3}) {
		t.Error("identical buffers should be equal")
	}
	if framebufferEqual(a, []uint32{1, 2, 4}) {
		t.Error("differing buffers should be unequal")
	}
	if framebufferEqual(a, []uint32{1, 2}) {
		t.Error("different-length buffers should be unequal")
	}
	if !framebufferEqual(nil, []uint32{}) {
		t.Error("empty buffers should be equal")
	}
}

// A freshly changed frame must be presented swapChainDepth times so every
// buffer in a multi-buffered swap chain holds it, then presentation stops.
func TestShouldPresentSettlesAfterStaticFrames(t *testing.T) {
	r := &Renderer{pixels: []uint32{0xAA, 0xBB, 0xCC}}
	for i := 0; i < swapChainDepth; i++ {
		if !r.shouldPresent() {
			t.Fatalf("present %d: want true while filling the swap chain", i)
		}
	}
	if r.shouldPresent() {
		t.Fatal("want false once every swap-chain buffer holds the frame")
	}
	if r.shouldPresent() {
		t.Fatal("want false to stay settled while the face is static")
	}
}

func TestShouldPresentRetriggersOnChange(t *testing.T) {
	r := &Renderer{pixels: []uint32{1, 1, 1}}
	for i := 0; i < swapChainDepth+2; i++ {
		r.shouldPresent() // settle
	}
	r.pixels = []uint32{1, 9, 1} // a single pixel changes
	for i := 0; i < swapChainDepth; i++ {
		if !r.shouldPresent() {
			t.Fatalf("post-change present %d: want true", i)
		}
	}
	if r.shouldPresent() {
		t.Fatal("want false after the change settles")
	}
}

// An arbitrary mod animation that changes pixels every frame must present every
// frame — no regression versus always-present.
func TestShouldPresentEveryFrameWhileAnimating(t *testing.T) {
	r := &Renderer{}
	for i := 0; i < 10; i++ {
		r.pixels = []uint32{uint32(i)}
		if !r.shouldPresent() {
			t.Fatalf("frame %d: want true while content changes every frame", i)
		}
	}
}

func TestShouldPresentResizeCountsAsChange(t *testing.T) {
	r := &Renderer{pixels: []uint32{1, 2, 3}}
	for i := 0; i < swapChainDepth+1; i++ {
		r.shouldPresent() // settle
	}
	r.pixels = []uint32{1, 2, 3, 4} // resize
	if !r.shouldPresent() {
		t.Fatal("a resize must count as a change and present")
	}
	if len(r.lastRendered) != len(r.pixels) {
		t.Fatalf("lastRendered len = %d, want %d", len(r.lastRendered), len(r.pixels))
	}
}

func TestFrameSignatureStaticVsAnimating(t *testing.T) {
	r := &Renderer{W: 1024, H: 768} // anims nil → not time-driven
	if got := r.frameSignature(FrameState{Expression: "neutral"}, "neutral", 0, 0); got != "neutral|1024|768" {
		t.Fatalf("static sig = %q, want neutral|1024|768", got)
	}
	// Every per-tick-animating case must yield "" (never skippable).
	if r.frameSignature(FrameState{Speaking: true}, "neutral", 0, 0) != "" {
		t.Error("speaking must not be skippable")
	}
	if r.frameSignature(FrameState{QuotaExhausted: true}, "neutral", 0, 0) != "" {
		t.Error("quota clock must not be skippable")
	}
	if r.frameSignature(FrameState{}, "sleeping", 0, 0) != "" {
		t.Error("sleeping (animated Z marks) must not be skippable")
	}
	ov := OverlayState{Visible: true}
	if r.frameSignature(FrameState{Overlay: &ov}, "neutral", 0, 0) != "" {
		t.Error("open overlay must not be skippable")
	}
	if r.frameSignature(FrameState{Toast: "RESTART REQUIRED"}, "neutral", 0, 0) != "" {
		t.Error("an active toast must not be skippable (it auto-dismisses)")
	}
}

func TestDrawToastDrawsCentredPanel(t *testing.T) {
	const w, h = int32(1024), int32(768) // TrimUI Brick resolution
	r := &Renderer{W: w, H: h, stride: int(w), pixels: make([]uint32, int(w*h))}
	r.drawToast(LayoutFor(w, h), "RESTART REQUIRED\nTO APPLY WAKE WORD")

	// The screen corner is outside the centred panel: untouched (background).
	if got := r.pixels[0]; got != 0 {
		t.Errorf("corner pixel = %#x, want 0 (panel must not cover the whole screen)", got)
	}
	// The screen centre is inside the panel: something was drawn there.
	if got := r.pixels[(h/2)*w+w/2]; got == 0 {
		t.Error("centre pixel is blank; the toast panel was not drawn")
	}
	// The panel is ~80% of the screen width: a left-edge column (outside the
	// centred 80% band) stays background, proving it is not full-bleed.
	if got := r.pixels[(h/2)*w+w/20]; got != 0 {
		t.Errorf("near-edge pixel = %#x, want 0 (panel should be ~80%% wide, not full width)", got)
	}
	// The panel is a modal, not a full repaint: only a subset of pixels are set.
	var set int
	for _, p := range r.pixels {
		if p != 0 {
			set++
		}
	}
	if set == 0 || set >= len(r.pixels) {
		t.Errorf("set pixels = %d/%d, want a centred subset", set, len(r.pixels))
	}
}

// stepStub is a minimal face.StepSource: it reports a fixed set of expressions
// as time-driven and returns a scripted step so frameSignature's time-driven
// branch can be exercised without building real animations.
type stepStub struct {
	timeDriven map[string]bool
	step       int
	ok         bool
}

func (s stepStub) IsTimeDriven(expr string) bool { return s.timeDriven[expr] }
func (s stepStub) FrameStep(expr string, w, h int, clock, epoch float64, signal float32) (int, bool) {
	return s.step, s.ok
}
func (s stepStub) AnimFrame(expr string, w, h int, clock, epoch float64, signal float32) ([]uint32, bool) {
	return nil, false
}

func TestFrameSignatureTimeDrivenFoldsStep(t *testing.T) {
	r := &Renderer{W: 1024, H: 768}
	r.anims = stepStub{timeDriven: map[string]bool{"look_around": true}, step: 3, ok: true}

	// A built time-driven face folds its current step into the signature, so a
	// held step stays skippable and an advanced step is a distinct signature.
	if got := r.frameSignature(FrameState{Expression: "look_around"}, "look_around", 1.0, 0); got != "look_around|step=3|1024|768" {
		t.Fatalf("time-driven sig = %q, want look_around|step=3|1024|768", got)
	}

	// Same expression, different step → different signature (forces a rebuild).
	r.anims = stepStub{timeDriven: map[string]bool{"look_around": true}, step: 4, ok: true}
	if got := r.frameSignature(FrameState{Expression: "look_around"}, "look_around", 1.25, 0); got == "look_around|step=3|1024|768" {
		t.Fatal("advancing the step must change the signature")
	}

	// Not yet built (ok=false) → "" so the static fallback keeps rebuilding.
	r.anims = stepStub{timeDriven: map[string]bool{"look_around": true}, step: 0, ok: false}
	if got := r.frameSignature(FrameState{Expression: "look_around"}, "look_around", 1.0, 0); got != "" {
		t.Fatalf("unbuilt time-driven sig = %q, want empty", got)
	}
}

func TestStaticFrameUnchanged(t *testing.T) {
	r := &Renderer{}
	if r.staticFrameUnchanged("") {
		t.Error("empty sig (animating) must never skip")
	}
	r.lastSig = "neutral|1024|768"
	r.dirtyPresents = 0
	if !r.staticFrameUnchanged("neutral|1024|768") {
		t.Error("matching static sig with drained swap chain should skip")
	}
	r.dirtyPresents = 1
	if r.staticFrameUnchanged("neutral|1024|768") {
		t.Error("must not skip while the swap chain is still filling")
	}
	r.dirtyPresents = 0
	if r.staticFrameUnchanged("smile|1024|768") {
		t.Error("a different static sig must not skip")
	}
}
