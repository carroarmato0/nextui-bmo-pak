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
