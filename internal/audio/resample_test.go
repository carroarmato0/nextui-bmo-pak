package audio

import (
	"encoding/binary"
	"math"
	"testing"
)

func sineS16LE(freqHz, sampleRate, frames int, amp float64) []byte {
	out := make([]byte, frames*2)
	for i := 0; i < frames; i++ {
		v := amp * math.Sin(2*math.Pi*float64(freqHz)*float64(i)/float64(sampleRate))
		binary.LittleEndian.PutUint16(out[i*2:], uint16(int16(v)))
	}
	return out
}

func zeroCrossings(pcm []byte) int {
	n := 0
	var prev int16
	for i := 0; i+1 < len(pcm); i += 2 {
		s := int16(binary.LittleEndian.Uint16(pcm[i:]))
		if (prev < 0 && s >= 0) || (prev >= 0 && s < 0) {
			n++
		}
		prev = s
	}
	return n
}

func TestResampleS16LESameRateUnchanged(t *testing.T) {
	pcm := sineS16LE(440, 16000, 160, 10000)
	got := ResampleS16LE(pcm, 16000, 16000, 1)
	if len(got) != len(pcm) {
		t.Fatalf("same-rate resample changed length: %d != %d", len(got), len(pcm))
	}
}

func TestResampleS16LEDurationPreserved(t *testing.T) {
	// 100ms at 24kHz -> 100ms at 16kHz.
	in := sineS16LE(600, 24000, 2400, 10000)
	out := ResampleS16LE(in, 24000, 16000, 1)
	if got, want := len(out)/2, 1600; got != want {
		t.Fatalf("out frames = %d, want %d (duration must be preserved)", got, want)
	}
}

func TestResampleS16LEPitchPreserved(t *testing.T) {
	// A 600Hz tone has 120 zero crossings in 100ms regardless of sample rate.
	in := sineS16LE(600, 24000, 2400, 10000)
	out := ResampleS16LE(in, 24000, 16000, 1)
	inZC, outZC := zeroCrossings(in), zeroCrossings(out)
	if diff := inZC - outZC; diff < -3 || diff > 3 {
		t.Fatalf("zero crossings in=%d out=%d; pitch not preserved", inZC, outZC)
	}
}

func TestResampleS16LEConstantSignal(t *testing.T) {
	in := make([]byte, 300*2)
	for i := 0; i < 300; i++ {
		binary.LittleEndian.PutUint16(in[i*2:], uint16(int16(1234)))
	}
	out := ResampleS16LE(in, 24000, 16000, 1)
	for i := 0; i+1 < len(out); i += 2 {
		if got := int16(binary.LittleEndian.Uint16(out[i:])); got != 1234 {
			t.Fatalf("constant signal distorted at frame %d: %d", i/2, got)
		}
	}
}

func TestResampleS16LETinyInput(t *testing.T) {
	// One frame plus a stray byte must not panic and must keep the frame.
	out := ResampleS16LE([]byte{0x10, 0x20, 0xFF}, 24000, 16000, 1)
	if len(out) != 2 {
		t.Fatalf("tiny input resampled to %d bytes, want 2", len(out))
	}
}
