package main

import (
	"testing"
	"time"
)

func TestParsePerfFlags(t *testing.T) {
	pf := parsePerfFlags([]string{
		"-cpuprofile", "/tmp/cpu.prof",
		"-memprofile", "/tmp/mem.prof",
		"-pprof", ":6060",
		"-perfsample", "/tmp/s.csv",
		"-perfinterval", "1s",
		"somepositional", "-unknownflag",
	})
	if pf.cpuProfile != "/tmp/cpu.prof" {
		t.Errorf("cpuProfile = %q", pf.cpuProfile)
	}
	if pf.memProfile != "/tmp/mem.prof" {
		t.Errorf("memProfile = %q", pf.memProfile)
	}
	if pf.pprofAddr != ":6060" {
		t.Errorf("pprofAddr = %q", pf.pprofAddr)
	}
	if pf.sampleFile != "/tmp/s.csv" {
		t.Errorf("sampleFile = %q", pf.sampleFile)
	}
	if pf.interval != time.Second {
		t.Errorf("interval = %s", pf.interval)
	}
}

func TestParsePerfFlagsDefaults(t *testing.T) {
	pf := parsePerfFlags(nil)
	if pf.cpuProfile != "" || pf.sampleFile != "" || pf.pprofAddr != "" {
		t.Errorf("expected empty paths, got %+v", pf)
	}
	if pf.interval != 2*time.Second {
		t.Errorf("default interval = %s, want 2s", pf.interval)
	}
	if pf.enabled() {
		t.Error("enabled() should be false with no flags")
	}
}
