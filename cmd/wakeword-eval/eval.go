package main

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
