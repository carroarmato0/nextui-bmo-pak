package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/carroarmato0/nextui-bmo/internal/audio"
)

// Options configures an evaluation run.
type Options struct {
	LibraryPath string
	MelModel    string
	EmbModel    string
	Model       string // candidate classifier .onnx
	Positives   string // dir of 16k mono WAVs that SHOULD wake
	Negatives   string // dir of 16k mono WAVs that should NOT wake
	Threshold   float64
	Threads     int
}

// Report holds evaluation results.
type Report struct {
	Positives        int
	PositiveAccepts  int
	Negatives        int
	NegativeSeconds  float64
	FalseAccepts     int
	TrueAcceptRate   float64 // 0..1
	FalseAcceptsHour float64
	SuggestedThresh  float64
	Separable        bool
}

// falseAcceptsPerHour scales a false-accept count over the negative audio
// duration to an hourly rate. Zero duration yields 0.
func falseAcceptsPerHour(falseAccepts int, negativeSeconds float64) float64 {
	if negativeSeconds <= 0 {
		return 0
	}
	return float64(falseAccepts) / negativeSeconds * 3600
}

// suggestThreshold proposes a decision threshold from per-clip max scores. If
// the lowest positive score exceeds the highest negative score the classes are
// separable and the midpoint is returned; otherwise ok is false.
func suggestThreshold(posMax, negMax []float64) (threshold float64, ok bool) {
	if len(posMax) == 0 {
		return 0, false
	}
	lowPos := minSlice(posMax)
	highNeg := 0.0
	if len(negMax) > 0 {
		highNeg = maxSlice(negMax)
	}
	if lowPos > highNeg {
		return (lowPos + highNeg) / 2, true
	}
	return 0, false
}

func minSlice(xs []float64) float64 {
	m := xs[0]
	for _, x := range xs[1:] {
		if x < m {
			m = x
		}
	}
	return m
}

func maxSlice(xs []float64) float64 {
	m := xs[0]
	for _, x := range xs[1:] {
		if x > m {
			m = x
		}
	}
	return m
}

type clip struct {
	name    string
	pcm     []byte // S16LE mono 16k
	seconds float64
}

// loadClips reads every *.wav in dir as 16 kHz mono S16LE, sorted by name.
func loadClips(dir string) ([]clip, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var clips []clip
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".wav" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		pcm, rate, ch, ok := audio.DecodeWAV(raw)
		if !ok {
			return nil, fmt.Errorf("decode wav %s", path)
		}
		if rate != 16000 || ch != 1 {
			return nil, fmt.Errorf("%s: want 16k mono, got rate=%d ch=%d", path, rate, ch)
		}
		clips = append(clips, clip{name: e.Name(), pcm: pcm, seconds: float64(len(pcm)/2) / 16000})
	}
	sort.Slice(clips, func(i, j int) bool { return clips[i].name < clips[j].name })
	return clips, nil
}
