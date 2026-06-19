package face

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	dir := t.TempDir()
	for _, n := range []string{"talk_0", "talk_1"} {
		if err := os.WriteFile(filepath.Join(dir, n+".svg"), []byte(tinyRedSVG), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	lib := NewLibraryMode(os.DirFS(dir), true)
	defs := map[string]AnimationDef{
		"talk": {Frames: []string{"talk_0", "talk_1"}, Driver: Driver{Kind: DriverAmplitude, Curve: "linear"}},
	}
	return NewEngine(lib, defs)
}

// testFrameDim is the square frame size used by the engine unit tests.
const testFrameDim = 4

func waitReady(t *testing.T, e *Engine, expr string) []uint32 {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if buf, ok := e.AnimFrame(expr, testFrameDim, testFrameDim, 0, 0, 1.0); ok {
			return buf
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("animation %q never became ready", expr)
	return nil
}

func TestEngineHasOnlyAnimated(t *testing.T) {
	e := newTestEngine(t)
	if !e.Has("talk") {
		t.Fatal("expected Has(talk)")
	}
	if e.Has("neutral") {
		t.Fatal("neutral is not animated")
	}
}

func TestEngineReturnsFalseForStatic(t *testing.T) {
	e := newTestEngine(t)
	if _, ok := e.AnimFrame("neutral", 4, 4, 0, 0, 0); ok {
		t.Fatal("static expr should return false")
	}
}

func TestEngineBuildsAndServesFrames(t *testing.T) {
	e := newTestEngine(t)
	buf := waitReady(t, e, "talk")
	if len(buf) != 16 {
		t.Fatalf("frame size=%d want 16", len(buf))
	}
}

func TestEngineReadyReflectsBuild(t *testing.T) {
	e := newTestEngine(t)
	if e.Ready("talk") {
		t.Fatal("not built yet")
	}
	waitReady(t, e, "talk")
	if !e.Ready("talk") {
		t.Fatal("should be ready after build")
	}
	if e.Ready("neutral") {
		t.Fatal("static expr is never ready")
	}
}

func TestEngineConcurrentAccessRaceClean(t *testing.T) {
	e := newTestEngine(t)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(k int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				expr := "talk"
				if k%2 == 0 {
					expr = "neutral"
				}
				e.AnimFrame(expr, 4, 4, float64(j)/10, 0, float32(j%2))
			}
		}(i)
	}
	wg.Wait()
}

// newMultiTestEngine builds an engine with three independent animated
// expressions (a, b, c), each a 2-frame amplitude animation.
func newMultiTestEngine(t *testing.T) *Engine {
	t.Helper()
	dir := t.TempDir()
	for _, n := range []string{"a_0", "a_1", "b_0", "b_1", "c_0", "c_1"} {
		if err := os.WriteFile(filepath.Join(dir, n+".svg"), []byte(tinyRedSVG), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	lib := NewLibraryMode(os.DirFS(dir), true)
	defs := map[string]AnimationDef{
		"a": {Frames: []string{"a_0", "a_1"}, Driver: Driver{Kind: DriverAmplitude, Curve: "linear"}},
		"b": {Frames: []string{"b_0", "b_1"}, Driver: Driver{Kind: DriverAmplitude, Curve: "linear"}},
		"c": {Frames: []string{"c_0", "c_1"}, Driver: Driver{Kind: DriverAmplitude, Curve: "linear"}},
	}
	return NewEngine(lib, defs)
}

// TestEngineKeepsExpressionsResidentAcrossSwitch guards the rebuild-gap fix:
// after building a second (and third) expression, an earlier one must STILL be
// resident, so re-entering it serves a frame immediately with no rebuild gap.
// The old single-slot engine evicted on every switch, which made the mouth lag
// the audio whenever the rendered expression changed.
func TestEngineKeepsExpressionsResidentAcrossSwitch(t *testing.T) {
	e := newMultiTestEngine(t)
	waitReady(t, e, "a")
	waitReady(t, e, "b")
	if !e.Ready("a") {
		t.Fatal("expr a evicted after building b — rebuild-gap regression")
	}
	if _, ok := e.AnimFrame("a", testFrameDim, testFrameDim, 0, 0, 1.0); !ok {
		t.Fatal("expr a frame not immediately available after switching to b")
	}
	waitReady(t, e, "c")
	if !e.Ready("b") {
		t.Fatal("expr b evicted after building c — rebuild-gap regression")
	}
}

// TestEnginePinnedSurvivesEviction guards the goodbye-lag fix: a pinned
// expression must stay resident no matter how many other animations are built
// after it, so the clip-backed talking face never rebuilds (and lags) on exit.
func TestEnginePinnedSurvivesEviction(t *testing.T) {
	dir := t.TempDir()
	names := []string{"p", "a", "b", "c", "d", "e"}
	for _, n := range names {
		for _, f := range []string{n + "_0", n + "_1"} {
			if err := os.WriteFile(filepath.Join(dir, f+".svg"), []byte(tinyRedSVG), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	lib := NewLibraryMode(os.DirFS(dir), true)
	defs := map[string]AnimationDef{}
	for _, n := range names {
		defs[n] = AnimationDef{Frames: []string{n + "_0", n + "_1"}, Driver: Driver{Kind: DriverAmplitude, Curve: "linear"}}
	}
	e := NewEngine(lib, defs)
	e.Pin("p")
	// Force a small cap so the unpinned builds below genuinely overflow it
	// (tiny test frames would otherwise leave the adaptive cap large).
	e.setCapForTest(4)
	waitReady(t, e, "p")
	// Build well past the cap so an unpinned entry would be evicted.
	for _, n := range []string{"a", "b", "c", "d", "e"} {
		waitReady(t, e, n)
	}
	if !e.Ready("p") {
		t.Fatal("pinned expr p was evicted — goodbye-lag regression")
	}
	if _, ok := e.AnimFrame("p", testFrameDim, testFrameDim, 0, 0, 1.0); !ok {
		t.Fatal("pinned expr p frame not immediately available")
	}
}

// TestSpeakingEmotionAnimates is the regression guard for WS1: with an emotion
// set and a positive amplitude signal, the engine must return an animated frame
// distinct from the rest frame. Before WS1, emotions had no animation defs so
// the mouth never moved while speaking.
func TestSpeakingEmotionAnimates(t *testing.T) {
	lib := NewLibrary(os.DirFS(t.TempDir()))
	eng := NewEngine(lib, DefaultAnimations())
	w, h := 80, 60
	eng.Prewarm(ExprNeutral, w, h)
	// Build is async; spin until ready (bounded).
	for i := 0; i < 200 && !eng.Ready(ExprNeutral); i++ {
		time.Sleep(5 * time.Millisecond)
	}
	if !eng.Ready(ExprNeutral) {
		t.Fatal("neutral animation never became ready")
	}
	rest, ok := eng.AnimFrame(ExprNeutral, w, h, 0, 0, 0) // silence
	if !ok {
		t.Fatal("rest AnimFrame not ok")
	}
	loud, ok := eng.AnimFrame(ExprNeutral, w, h, 0, 0, 1.0) // full voice
	if !ok {
		t.Fatal("loud AnimFrame not ok")
	}
	if equalFrame(rest, loud) {
		t.Fatal("mouth did not move between silence and full voice (regression)")
	}
}

// TestIsAmplitude verifies the render-loop gate that applies the inter-syllable
// mouth floor: every default emotion face (and speaking) is amplitude-driven,
// time-driven idle faces are not, and unknown faces report false.
func TestIsAmplitude(t *testing.T) {
	eng := NewEngine(NewLibrary(os.DirFS(t.TempDir())), DefaultAnimations())
	for _, expr := range []string{ExprNeutral, ExprHappy, ExprUnamused, ExprSkeptical, ExprAngry, ExprSpeaking} {
		if !eng.IsAmplitude(expr) {
			t.Errorf("IsAmplitude(%q) = false, want true (amplitude lip-sync face)", expr)
		}
	}
	for _, expr := range []string{ExprLookAround, ExprWhistle, ExprSleeping} {
		if eng.IsAmplitude(expr) {
			t.Errorf("IsAmplitude(%q) = true, want false (time-driven idle face)", expr)
		}
	}
	if eng.IsAmplitude("no-such-face") {
		t.Error("IsAmplitude(unknown) = true, want false")
	}
}

// newTimeDrivenEngine builds an engine with a two-frame, time-driven "loop"
// animation whose frames are visually distinct, so a step index can be
// correlated to the pixels AnimFrame returns.
func newTimeDrivenEngine(t *testing.T) *Engine {
	t.Helper()
	const greenSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><rect x="0" y="0" width="10" height="10" fill="#00ff00"/></svg>`
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "look_0.svg"), []byte(tinyRedSVG), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "look_1.svg"), []byte(greenSVG), 0o644); err != nil {
		t.Fatal(err)
	}
	lib := NewLibraryMode(os.DirFS(dir), true)
	defs := map[string]AnimationDef{
		// FPS 4, loop: step = int(clock*4) % 2.
		"look": {Frames: []string{"look_0", "look_1"}, Driver: Driver{Kind: DriverTime, FPS: 4, Mode: modeLoop}},
	}
	return NewEngine(lib, defs)
}

func TestFrameStepFalseForStatic(t *testing.T) {
	e := newTimeDrivenEngine(t)
	if _, ok := e.FrameStep("neutral", testFrameDim, testFrameDim, 0, 0, 0); ok {
		t.Fatal("FrameStep should report false for a static expression")
	}
}

func TestFrameStepFalseBeforeBuilt(t *testing.T) {
	e := newTimeDrivenEngine(t)
	// No AnimFrame call yet, so nothing is built: FrameStep must not invent a step.
	if _, ok := e.FrameStep("look", testFrameDim, testFrameDim, 0, 0, 0); ok {
		t.Fatal("FrameStep should report false before the animation is built")
	}
}

func TestFrameStepDoesNotTriggerBuild(t *testing.T) {
	e := newTimeDrivenEngine(t)
	for i := 0; i < 10; i++ {
		e.FrameStep("look", testFrameDim, testFrameDim, 0, 0, 0)
	}
	// FrameStep is a pure peek: it must never kick off a build the way AnimFrame does.
	if e.Ready("look") {
		t.Fatal("FrameStep must not start a build")
	}
}

func TestFrameStepAgreesWithAnimFrame(t *testing.T) {
	e := newTimeDrivenEngine(t)
	waitReady(t, e, "look") // builds at testFrameDim via AnimFrame

	// Across a range of clocks, the frame AnimFrame shows must be exactly the
	// frame at the index FrameStep reports — and held clocks must report a
	// stable step.
	for _, clock := range []float64{0, 0.1, 0.24, 0.25, 0.49, 0.5, 0.75, 1.0} {
		step, ok := e.FrameStep("look", testFrameDim, testFrameDim, clock, 0, 0)
		if !ok {
			t.Fatalf("FrameStep not ok at clock=%v", clock)
		}
		buf, ok := e.AnimFrame("look", testFrameDim, testFrameDim, clock, 0, 0)
		if !ok {
			t.Fatalf("AnimFrame not ok at clock=%v", clock)
		}
		// Re-fetch the frame at the reported step directly to confirm agreement.
		ref, ok := e.AnimFrame("look", testFrameDim, testFrameDim, float64(step)/4+0.01, 0, 0)
		if !ok {
			t.Fatalf("reference AnimFrame not ok for step=%d", step)
		}
		if !equalFrame(buf, ref) {
			t.Fatalf("clock=%v: FrameStep=%d disagrees with the frame AnimFrame returned", clock, step)
		}
	}
}
