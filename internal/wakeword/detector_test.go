package wakeword

import (
	"os"
	"testing"

	"github.com/carroarmato0/nextui-bmo/internal/audio"
)

// Env-gated so CI without the ORT .so and models can skip:
//
//	ONNXRUNTIME_LIB      path to libonnxruntime.so
//	WAKEWORD_MEL         path to melspectrogram.onnx
//	WAKEWORD_EMB         path to embedding_model.onnx
//	WAKEWORD_WAKE        path to a wake classifier (e.g. alexa_v0.1.onnx)
//	WAKEWORD_POSITIVE    path to a 16k mono S16LE WAV that should fire WAKEWORD_WAKE
func testConfig(t *testing.T) Config {
	t.Helper()
	lib := os.Getenv("ONNXRUNTIME_LIB")
	mel := os.Getenv("WAKEWORD_MEL")
	emb := os.Getenv("WAKEWORD_EMB")
	wake := os.Getenv("WAKEWORD_WAKE")
	if lib == "" || mel == "" || emb == "" || wake == "" {
		t.Skip("set ONNXRUNTIME_LIB, WAKEWORD_MEL, WAKEWORD_EMB, WAKEWORD_WAKE to run")
	}
	return Config{LibraryPath: lib, MelModel: mel, EmbModel: emb, WakeModel: wake, Threshold: 0.5, Threads: 2}
}

func mustReadWAV(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read wav: %v", err)
	}
	pcm, rate, ch, ok := audio.DecodeWAV(raw)
	if !ok {
		t.Fatalf("decode wav %s", path)
	}
	if rate != 16000 || ch != 1 {
		t.Fatalf("expected 16k mono, got rate=%d ch=%d", rate, ch)
	}
	return pcm
}

func silence(seconds float64) []byte {
	return make([]byte, int(seconds*16000)*2)
}

func chunk(b []byte, n int) [][]byte {
	var out [][]byte
	for len(b) >= n {
		out = append(out, b[:n])
		b = b[n:]
	}
	if len(b) > 0 {
		out = append(out, b)
	}
	return out
}

func TestDetectorPositiveClipFires(t *testing.T) {
	cfg := testConfig(t)
	pos := os.Getenv("WAKEWORD_POSITIVE")
	if pos == "" {
		t.Skip("set WAKEWORD_POSITIVE to a clip that should fire WAKEWORD_WAKE")
	}
	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer d.Close()

	// Pad with leading/trailing silence so the short clip has enough context to
	// fill the classifier window.
	stream := append(silence(1.5), mustReadWAV(t, pos)...)
	stream = append(stream, silence(0.5)...)

	var maxScore float64
	for _, frame := range chunk(stream, hopSamples*2) {
		for _, det := range d.Write(frame) {
			if det.Score > maxScore {
				maxScore = det.Score
			}
		}
	}
	if maxScore < 0.5 {
		t.Fatalf("expected wake on positive clip; max score %.3f", maxScore)
	}
	t.Logf("positive clip max score %.3f", maxScore)
}

func TestDetectorSilenceDoesNotFire(t *testing.T) {
	cfg := testConfig(t)
	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer d.Close()
	for _, frame := range chunk(silence(3), hopSamples*2) {
		for _, det := range d.Write(frame) {
			if det.Score >= 0.5 {
				t.Fatalf("false trigger on silence: %.3f", det.Score)
			}
		}
	}
}

func TestDetectorRefractorySuppressesRepeat(t *testing.T) {
	// Pure-logic guard: a fresh detector starts past the refractory window so a
	// first crossing fires, then is suppressed for RefractorySteps hops. We
	// exercise step bookkeeping without ONNX by faking scores is not possible
	// here (runCls needs models), so this is covered by the controller tests;
	// keep this as a smoke check that Reset restores the refractory gate.
	d := &Detector{refractory: defaultRefractorySteps, sinceFired: 0}
	d.embBuf = make([]float32, classWindow*embDim)
	d.embCount = classWindow
	d.Reset()
	if d.sinceFired != defaultRefractorySteps {
		t.Fatalf("Reset should arm the refractory gate, got %d", d.sinceFired)
	}
}
