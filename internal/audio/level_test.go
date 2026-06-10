package audio

import "testing"

func TestPCMLevelS16LE(t *testing.T) {
	if got := PCMLevelS16LE(nil); got != 0 {
		t.Fatalf("PCMLevelS16LE(nil) = %v, want 0", got)
	}

	quiet := []byte{0x00, 0x00, 0x00, 0x00}
	if got := PCMLevelS16LE(quiet); got != 0 {
		t.Fatalf("PCMLevelS16LE(quiet) = %v, want 0", got)
	}

	loud := []byte{0x00, 0x40, 0x00, 0x40}
	if got := PCMLevelS16LE(loud); got <= 0 {
		t.Fatalf("PCMLevelS16LE(loud) = %v, want > 0", got)
	}
	if !PCMHasSignal(loud, 0.05) {
		t.Fatal("PCMHasSignal(loud) = false, want true")
	}
}

func TestBytesPerSecond(t *testing.T) {
	if got := BytesPerSecond(16000, 1, 2); got != 32000 {
		t.Fatalf("BytesPerSecond() = %d, want 32000", got)
	}
	if got := BytesPerSecond(0, 1, 2); got != 0 {
		t.Fatalf("BytesPerSecond(0, 1, 2) = %d, want 0", got)
	}
}
