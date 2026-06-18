package perf

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStartCPUProfileWritesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cpu.prof")
	stop, err := StartCPUProfile(path)
	if err != nil {
		t.Fatalf("StartCPUProfile: %v", err)
	}
	// Burn a little CPU so the profile has at least one sample.
	x := 0
	for i := 0; i < 5_000_000; i++ {
		x += i
	}
	_ = x
	stop()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat profile: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("cpu profile is empty")
	}
}

func TestWriteHeapProfileWritesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "heap.prof")
	if err := WriteHeapProfile(path); err != nil {
		t.Fatalf("WriteHeapProfile: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat profile: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("heap profile is empty")
	}
}
