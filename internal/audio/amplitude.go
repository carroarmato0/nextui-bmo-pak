package audio

import "sort"

// peakRefPercentile is the amplitude percentile mapped to full mouth-open by
// NormalizePeakRMS. Using a high percentile (rather than the absolute max) means
// a single anomalously loud chunk cannot scale the rest of the utterance down,
// and the loudest ~5% of speech saturates the mouth wide open.
const peakRefPercentile = 0.95

// silenceFloor guards against amplifying a silent or near-silent utterance into
// full-amplitude noise: below this reference RMS the input is returned unscaled.
const silenceFloor = 0.01

// NormalizePeakRMS scales a sequence of per-chunk RMS amplitudes so an
// utterance's loud passages reach full mouth-open regardless of the source's
// absolute volume — a quiet TTS voice (e.g. Kokoro) and a loud one (e.g. OpenAI)
// both drive the full range of the lip-sync animation. The reference is a high
// percentile (peakRefPercentile) so outliers don't suppress the body, and
// results are clamped to [0,1]. An empty or near-silent input is returned
// unchanged.
func NormalizePeakRMS(amps []float32) []float32 {
	if len(amps) == 0 {
		return amps
	}
	sorted := make([]float32, len(amps))
	copy(sorted, amps)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	idx := int(peakRefPercentile*float64(len(sorted)-1) + 0.5)
	ref := sorted[idx]
	if ref < silenceFloor {
		return amps
	}

	scale := 1.0 / ref
	out := make([]float32, len(amps))
	for i, v := range amps {
		s := v * scale
		if s > 1 {
			s = 1
		} else if s < 0 {
			s = 0
		}
		out[i] = s
	}
	return out
}
