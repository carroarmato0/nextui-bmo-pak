package audio

import (
	"math"
	"testing"
)

func TestS16LEToFloat32(t *testing.T) {
	// 0x0000 -> 0, 0x7FFF -> ~+1, 0x8000 -> -1
	pcm := []byte{0x00, 0x00, 0xFF, 0x7F, 0x00, 0x80}
	got := S16LEToFloat32(pcm)
	want := []float32{0, 32767.0 / 32768.0, -1}
	if len(got) != len(want) {
		t.Fatalf("len=%d want %d", len(got), len(want))
	}
	for i := range want {
		if math.Abs(float64(got[i]-want[i])) > 1e-6 {
			t.Fatalf("sample %d = %v want %v", i, got[i], want[i])
		}
	}
}

func TestS16LEToFloat32OddBytesIgnoresTrailing(t *testing.T) {
	if got := S16LEToFloat32([]byte{0x00, 0x00, 0x01}); len(got) != 1 {
		t.Fatalf("len=%d want 1 (trailing odd byte dropped)", len(got))
	}
}
