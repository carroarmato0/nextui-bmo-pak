package audio

import "encoding/binary"

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
