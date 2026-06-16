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
	lib := NewLibraryMode(dir, true)
	defs := map[string]AnimationDef{
		"talk": {Frames: []string{"talk_0", "talk_1"}, Driver: Driver{Kind: DriverAmplitude, Curve: "linear"}},
	}
	return NewEngine(lib, defs)
}

func waitReady(t *testing.T, e *Engine, expr string, w, h int) []uint32 {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if buf, ok := e.AnimFrame(expr, w, h, 0, 0, 1.0); ok {
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
	buf := waitReady(t, e, "talk", 4, 4)
	if len(buf) != 16 {
		t.Fatalf("frame size=%d want 16", len(buf))
	}
}

func TestEngineReadyReflectsBuild(t *testing.T) {
	e := newTestEngine(t)
	if e.Ready("talk") {
		t.Fatal("not built yet")
	}
	waitReady(t, e, "talk", 4, 4)
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

// TestSpeakingEmotionAnimates is the regression guard for WS1: with an emotion
// set and a positive amplitude signal, the engine must return an animated frame
// distinct from the rest frame. Before WS1, emotions had no animation defs so
// the mouth never moved while speaking.
func TestSpeakingEmotionAnimates(t *testing.T) {
	lib := NewLibrary(t.TempDir())
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
