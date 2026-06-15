package audio

import (
	"encoding/binary"
	"math"
)

// ResampleS16LE converts interleaved 16-bit little-endian PCM from one sample
// rate to another using linear interpolation. The quality is adequate for
// speech on small speakers. The input is returned unchanged when no
// conversion is needed or the arguments are invalid; a trailing partial frame
// is dropped.
func ResampleS16LE(pcm []byte, fromRate, toRate, channels int) []byte {
	if fromRate <= 0 || toRate <= 0 || channels <= 0 || fromRate == toRate {
		return pcm
	}
	frameBytes := channels * 2
	inFrames := len(pcm) / frameBytes
	if inFrames == 0 {
		return nil
	}
	outFrames := (inFrames*toRate + fromRate/2) / fromRate
	if outFrames < 1 {
		outFrames = 1
	}
	out := make([]byte, outFrames*frameBytes)
	step := float64(fromRate) / float64(toRate)
	for i := 0; i < outFrames; i++ {
		pos := float64(i) * step
		i0 := int(pos)
		if i0 > inFrames-1 {
			i0 = inFrames - 1
		}
		i1 := i0 + 1
		if i1 > inFrames-1 {
			i1 = inFrames - 1
		}
		frac := pos - float64(i0)
		if frac < 0 {
			frac = 0
		} else if frac > 1 {
			frac = 1
		}
		for ch := 0; ch < channels; ch++ {
			s0 := float64(int16(binary.LittleEndian.Uint16(pcm[(i0*channels+ch)*2:])))
			s1 := float64(int16(binary.LittleEndian.Uint16(pcm[(i1*channels+ch)*2:])))
			v := s0 + (s1-s0)*frac
			binary.LittleEndian.PutUint16(out[(i*channels+ch)*2:], uint16(int16(v)))
		}
	}
	return out
}

// UpmixMonoToStereo duplicates each S16LE sample into an interleaved L+R pair,
// converting mono PCM to stereo without altering the sample rate or pitch.
// A trailing odd byte is dropped. Returns pcm unchanged if it is empty.
func UpmixMonoToStereo(pcm []byte) []byte {
	if len(pcm) == 0 {
		return pcm
	}
	stereo := make([]byte, (len(pcm)/2)*4)
	for i := 0; i+1 < len(pcm); i += 2 {
		s := binary.LittleEndian.Uint16(pcm[i : i+2])
		j := i * 2
		binary.LittleEndian.PutUint16(stereo[j:j+2], s)
		binary.LittleEndian.PutUint16(stereo[j+2:j+4], s)
	}
	return stereo
}

// RMSChunks splits pcm into chunkMs-millisecond windows of S16LE interleaved
// PCM and returns the RMS amplitude [0, 1] of each window.
func RMSChunks(pcm []byte, sampleRate, channels, chunkMs int) []float32 {
	if sampleRate <= 0 || channels <= 0 || chunkMs <= 0 {
		return nil
	}
	bytesPerChunk := sampleRate * channels * 2 * chunkMs / 1000
	if bytesPerChunk <= 0 {
		return nil
	}
	var out []float32
	for i := 0; i+bytesPerChunk <= len(pcm); i += bytesPerChunk {
		chunk := pcm[i : i+bytesPerChunk]
		var sum float64
		n := 0
		for j := 0; j+1 < len(chunk); j += 2 {
			v := float64(int16(binary.LittleEndian.Uint16(chunk[j:j+2]))) / 32767.0
			sum += v * v
			n++
		}
		if n > 0 {
			out = append(out, float32(math.Sqrt(sum/float64(n))))
		} else {
			out = append(out, 0)
		}
	}
	return out
}
