package audio

import (
	"encoding/binary"
	"math"
)

const BytesPerSampleS16LE = 2

func BytesPerSecond(sampleRate, channels, bytesPerSample int) int {
	if sampleRate <= 0 || channels <= 0 || bytesPerSample <= 0 {
		return 0
	}
	return sampleRate * channels * bytesPerSample
}

func PCMLevelS16LE(pcm []byte) float64 {
	if len(pcm) < BytesPerSampleS16LE {
		return 0
	}

	var total float64
	var samples int
	for i := 0; i+1 < len(pcm); i += 2 {
		sample := int16(binary.LittleEndian.Uint16(pcm[i : i+2]))
		total += math.Abs(float64(sample))
		samples++
	}
	if samples == 0 {
		return 0
	}
	return total / float64(samples) / 32768.0
}

func PCMHasSignal(pcm []byte, threshold float64) bool {
	if threshold <= 0 {
		return len(pcm) > 0
	}
	return PCMLevelS16LE(pcm) >= threshold
}
