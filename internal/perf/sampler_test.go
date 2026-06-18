package perf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseVmRSSKB(t *testing.T) {
	const status = `VmPeak:	  123456 kB
VmSize:	  120000 kB
VmRSS:	   45678 kB
VmData:	   10000 kB
`
	got, err := parseVmRSSKB(strings.NewReader(status))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 45678 {
		t.Fatalf("VmRSS = %d, want 45678", got)
	}
}

func TestParseVmRSSKBMissing(t *testing.T) {
	if _, err := parseVmRSSKB(strings.NewReader("VmSize:\t100 kB\n")); err == nil {
		t.Fatal("expected error when VmRSS absent")
	}
}

func TestParseProcStatCPU(t *testing.T) {
	// comm "(bmo pak)" deliberately contains a space and parens-adjacent text
	// to exercise last-')' splitting. Fields after state: utime is field 14,
	// stime field 15 (1-indexed in proc(5)).
	stat := "4242 (bmo pak) R 1 4242 4242 0 -1 0 0 0 0 0 " +
		"731 412 0 0 20 0 8 0 99999 1234 567 rest ignored\n"
	utime, stime, err := parseProcStatCPU(strings.NewReader(stat))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if utime != 731 || stime != 412 {
		t.Fatalf("utime,stime = %d,%d want 731,412", utime, stime)
	}
}

func TestCPUPercent(t *testing.T) {
	// 50 ticks over 1s at 100 ticks/s = 0.5 CPU-seconds = 50%.
	got := cpuPercent(100, 150, time.Second)
	if got < 49.9 || got > 50.1 {
		t.Fatalf("cpuPercent = %f, want ~50", got)
	}
	// Zero wall duration must not divide-by-zero.
	if v := cpuPercent(100, 150, 0); v != 0 {
		t.Fatalf("cpuPercent(zero wall) = %f, want 0", v)
	}
}

func TestFormatRow(t *testing.T) {
	row := formatRow(sampleRow{
		uptimeS:      12.5,
		state:        "speaking",
		vmrssKB:      45678,
		goHeapAllocK: 2048,
		goHeapSysK:   8192,
		goNumGC:      7,
		cpuPct:       33.3,
		goroutines:   42,
	})
	want := "12.50,speaking,45678,2048,8192,7,33.30,42\n"
	if row != want {
		t.Fatalf("formatRow = %q, want %q", row, want)
	}
}

func TestSamplerWritesHeaderAndRows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "perf-sample.csv")
	s := NewSampler(path, 10*time.Millisecond, func() string { return "idle" }, testLogger{})
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(45 * time.Millisecond)
	s.Stop()
	s.Stop() // idempotent

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if lines[0] != csvHeader {
		t.Fatalf("header = %q, want %q", lines[0], csvHeader)
	}
	if len(lines) < 3 { // header + first immediate row + at least one tick + final
		t.Fatalf("expected several rows, got %d:\n%s", len(lines), data)
	}
	if !strings.Contains(lines[1], ",idle,") {
		t.Fatalf("row missing state tag: %q", lines[1])
	}
}

type testLogger struct{}

func (testLogger) Infof(string, ...any)  {}
func (testLogger) Errorf(string, ...any) {}

func TestSamplerStopWithoutSuccessfulStart(t *testing.T) {
	// Stop must not panic or deadlock when Start was never called (or failed):
	// no goroutine was launched and no file was opened.
	s := NewSampler(filepath.Join(t.TempDir(), "x.csv"), time.Second, nil, testLogger{})
	done := make(chan struct{})
	go func() { s.Stop(); s.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop deadlocked when Start never succeeded")
	}
}
