package main

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestFalseAcceptsPerHour(t *testing.T) {
	got := falseAcceptsPerHour(2, 1800) // 2 in half an hour -> 4/hr
	if math.Abs(got-4) > 1e-9 {
		t.Fatalf("got %v want 4", got)
	}
	if falseAcceptsPerHour(5, 0) != 0 {
		t.Fatal("zero duration should yield 0")
	}
}

func TestSuggestThresholdSeparable(t *testing.T) {
	th, ok := suggestThreshold([]float64{0.8, 0.9}, []float64{0.2, 0.3})
	if !ok {
		t.Fatal("expected separable")
	}
	if math.Abs(th-0.55) > 1e-9 { // midpoint of lowestPos(0.8) and highestNeg(0.3)
		t.Fatalf("got %v want 0.55", th)
	}
}

func TestSuggestThresholdOverlap(t *testing.T) {
	if _, ok := suggestThreshold([]float64{0.4}, []float64{0.6}); ok {
		t.Fatal("expected not separable")
	}
	if _, ok := suggestThreshold(nil, []float64{0.1}); ok {
		t.Fatal("no positives -> not separable")
	}
}

// writeWAV writes a minimal 16-bit PCM mono WAV with the given sample rate.
func writeWAV(t *testing.T, path string, sampleRate int, samples []int16) {
	t.Helper()
	var b []byte
	put := func(s string) { b = append(b, s...) }
	u32 := func(v uint32) { var x [4]byte; binary.LittleEndian.PutUint32(x[:], v); b = append(b, x[:]...) }
	u16 := func(v uint16) { var x [2]byte; binary.LittleEndian.PutUint16(x[:], v); b = append(b, x[:]...) }
	dataLen := uint32(len(samples) * 2)
	put("RIFF")
	u32(36 + dataLen)
	put("WAVE")
	put("fmt ")
	u32(16)              // fmt chunk size
	u16(1)               // PCM
	u16(1)               // mono
	u32(uint32(sampleRate))
	u32(uint32(sampleRate * 2)) // byte rate
	u16(2)               // block align
	u16(16)              // bits/sample
	put("data")
	u32(dataLen)
	for _, s := range samples {
		u16(uint16(s))
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("write wav: %v", err)
	}
}

func TestLoadClips(t *testing.T) {
	dir := t.TempDir()
	writeWAV(t, filepath.Join(dir, "b.wav"), 16000, make([]int16, 16000)) // 1.0 s
	writeWAV(t, filepath.Join(dir, "a.wav"), 16000, make([]int16, 8000))  // 0.5 s
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	clips, err := loadClips(dir)
	if err != nil {
		t.Fatalf("loadClips: %v", err)
	}
	if len(clips) != 2 {
		t.Fatalf("want 2 clips, got %d", len(clips))
	}
	if clips[0].name != "a.wav" || clips[1].name != "b.wav" {
		t.Fatalf("clips not sorted by name: %v %v", clips[0].name, clips[1].name)
	}
	if clips[0].seconds < 0.49 || clips[0].seconds > 0.51 {
		t.Fatalf("a.wav seconds %v want ~0.5", clips[0].seconds)
	}
}

func TestLoadClipsRejectsWrongRate(t *testing.T) {
	dir := t.TempDir()
	writeWAV(t, filepath.Join(dir, "bad.wav"), 44100, make([]int16, 4410))
	if _, err := loadClips(dir); err == nil {
		t.Fatal("expected error on non-16k WAV")
	}
}
