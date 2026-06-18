package perf

import (
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
