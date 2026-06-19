package audio

import (
	"math"
	"testing"
)

func TestNormalizePeakRMS(t *testing.T) {
	t.Run("empty stays empty", func(t *testing.T) {
		if got := NormalizePeakRMS(nil); got != nil {
			t.Fatalf("got %v, want nil", got)
		}
	})

	t.Run("silence is left unchanged", func(t *testing.T) {
		in := []float32{0, 0, 0, 0}
		out := NormalizePeakRMS(in)
		for i, v := range out {
			if v != 0 {
				t.Fatalf("out[%d] = %v, want 0 (silent input must not be amplified)", i, v)
			}
		}
	})

	t.Run("quiet utterance reaches full open at its loud passages", func(t *testing.T) {
		// Kokoro-like: peaks ~0.18, never near 1.0.
		in := []float32{0.02, 0.06, 0.10, 0.14, 0.18, 0.05}
		out := NormalizePeakRMS(in)
		var max float32
		for _, v := range out {
			if v > max {
				max = v
			}
			if v < 0 || v > 1 {
				t.Fatalf("normalized value %v out of [0,1]", v)
			}
		}
		if max < 0.99 {
			t.Fatalf("max normalized = %.3f, want ~1.0 (loud passages should reach full open)", max)
		}
	})

	t.Run("monotonic: louder input stays louder after normalize", func(t *testing.T) {
		in := []float32{0.02, 0.08, 0.16}
		out := NormalizePeakRMS(in)
		for i := 1; i < len(out); i++ {
			if out[i] < out[i-1] {
				t.Fatalf("normalization not monotonic: out=%v", out)
			}
		}
	})

	t.Run("single outlier does not suppress the body", func(t *testing.T) {
		// One anomalous spike; the bulk sits around 0.1. A max-based scale would
		// crush the body; a high-percentile reference keeps it lively.
		in := make([]float32, 0, 100)
		for i := 0; i < 99; i++ {
			in = append(in, 0.10)
		}
		in = append(in, 0.90) // lone spike
		out := NormalizePeakRMS(in)
		if out[0] < 0.9 {
			t.Fatalf("body normalized to %.3f, want >=0.9 (outlier must not suppress the body)", out[0])
		}
		if math.IsNaN(float64(out[0])) {
			t.Fatalf("NaN in output")
		}
	})
}
