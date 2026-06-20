package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/carroarmato0/nextui-bmo/internal/audio"
	"github.com/carroarmato0/nextui-bmo/internal/wakeword"
)

const (
	hopBytes      = 1280 * 2 // one 80 ms S16LE hop
	scanThreshold = 1e-6     // near-zero so the scan pass reports every hop's score
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

// chunkPCM splits b into n-byte frames (last frame may be shorter).
func chunkPCM(b []byte, n int) [][]byte {
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

// pad prepends 1.0 s and appends 0.5 s of silence so short clips have enough
// leading context to fill the classifier window (~1.28 s) before the utterance,
// mirroring how the on-device detector meets a wake phrase inside a live stream.
func pad(pcm []byte) []byte {
	lead := make([]byte, int(1.0*16000)*2) // 1.0 s
	tail := make([]byte, int(0.5*16000)*2) // 0.5 s
	out := make([]byte, 0, len(lead)+len(pcm)+len(tail))
	out = append(out, lead...)
	out = append(out, pcm...)
	out = append(out, tail...)
	return out
}

// clipFires reports whether the detector fires at least once over pcm.
func clipFires(d *wakeword.Detector, pcm []byte) bool {
	d.Reset()
	fired := false
	for _, frame := range chunkPCM(pcm, hopBytes) {
		if len(d.Write(frame)) > 0 {
			fired = true
		}
	}
	return fired
}

// countFires totals detector firings over pcm (refractory-suppressed, like
// on-device), used to count false accepts.
func countFires(d *wakeword.Detector, pcm []byte) int {
	d.Reset()
	n := 0
	for _, frame := range chunkPCM(pcm, hopBytes) {
		n += len(d.Write(frame))
	}
	return n
}

// maxScores returns each clip's peak hop score using a low-threshold,
// refractory-1 detector so every hop reports.
func maxScores(d *wakeword.Detector, clips []clip) []float64 {
	out := make([]float64, 0, len(clips))
	for _, c := range clips {
		d.Reset()
		m := 0.0
		for _, frame := range chunkPCM(pad(c.pcm), hopBytes) {
			for _, det := range d.Write(frame) {
				if det.Score > m {
					m = det.Score
				}
			}
		}
		out = append(out, m)
	}
	return out
}

// Run evaluates Options.Model against the positive/negative folders. It first
// asserts the model satisfies the classifier contract.
func Run(o Options) (Report, error) {
	if err := wakeword.InitEnv(o.LibraryPath); err != nil {
		return Report{}, err
	}
	if err := wakeword.ValidateClassifier(o.Model); err != nil {
		return Report{}, fmt.Errorf("contract check: %w", err)
	}
	pos, err := loadClips(o.Positives)
	if err != nil {
		return Report{}, fmt.Errorf("positives: %w", err)
	}
	neg, err := loadClips(o.Negatives)
	if err != nil {
		return Report{}, fmt.Errorf("negatives: %w", err)
	}

	base := wakeword.Config{LibraryPath: o.LibraryPath, MelModel: o.MelModel, EmbModel: o.EmbModel, WakeModel: o.Model, Threads: o.Threads}

	atCfg := base
	atCfg.Threshold = o.Threshold
	det, err := wakeword.New(atCfg)
	if err != nil {
		return Report{}, err
	}
	defer det.Close()

	rep := Report{Positives: len(pos), Negatives: len(neg)}
	for _, c := range pos {
		if clipFires(det, pad(c.pcm)) {
			rep.PositiveAccepts++
		}
	}
	for _, c := range neg {
		rep.FalseAccepts += countFires(det, c.pcm)
		rep.NegativeSeconds += c.seconds
	}
	if rep.Positives > 0 {
		rep.TrueAcceptRate = float64(rep.PositiveAccepts) / float64(rep.Positives)
	}
	rep.FalseAcceptsHour = falseAcceptsPerHour(rep.FalseAccepts, rep.NegativeSeconds)

	scanCfg := base
	scanCfg.Threshold = scanThreshold
	scanCfg.RefractorySteps = 1
	scan, err := wakeword.New(scanCfg)
	if err != nil {
		return Report{}, err
	}
	defer scan.Close()
	rep.SuggestedThresh, rep.Separable = suggestThreshold(maxScores(scan, pos), maxScores(scan, neg))
	return rep, nil
}
