package devctx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSystemCollector(t *testing.T) {
	dir := t.TempDir()
	uptime := filepath.Join(dir, "uptime")
	meminfo := filepath.Join(dir, "meminfo")
	powerDir := filepath.Join(dir, "power_supply")
	if err := os.WriteFile(uptime, []byte("242883.21 423310.42\n"), 0o600); err != nil { // 2d19h
		t.Fatal(err)
	}
	if err := os.WriteFile(meminfo, []byte("MemTotal:        998332 kB\nMemFree:         511784 kB\nMemAvailable:    680408 kB\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(powerDir, "axp2202-battery"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(powerDir, "axp2202-battery", "capacity"), []byte("84\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	c := SystemCollector{
		Model:       "TrimUI Brick",
		UptimePath:  uptime,
		MeminfoPath: meminfo,
		DiskPath:    dir, // real statfs on the temp dir
		PowerDir:    powerDir,
	}
	s, err := c.Collect(time.Now())
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if s.Key != KeySystem || s.Title != "YOUR BODY (THE DEVICE)" {
		t.Fatalf("unexpected section identity: %+v", s)
	}
	for _, want := range []string{
		"TrimUI Brick",
		"awake for 2 days and 19 hours",
		"Memory is 32% used", // 1 - 680408/998332 ≈ 31.8 → rounds to 32
		"SD card:",
		"Battery is at 84%",
	} {
		if !strings.Contains(s.Body, want) {
			t.Errorf("body missing %q: %q", want, s.Body)
		}
	}
	if !s.Freshest.IsZero() {
		t.Errorf("system section is evergreen; got Freshest %v", s.Freshest)
	}
}

func TestSystemCollectorPartialSources(t *testing.T) {
	// Only the model is known: still produces a section.
	s, err := (SystemCollector{Model: "TrimUI Brick"}).Collect(time.Now())
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if !strings.Contains(s.Body, "TrimUI Brick") {
		t.Errorf("missing model: %q", s.Body)
	}
}

func TestSystemCollectorNothingAvailable(t *testing.T) {
	if _, err := (SystemCollector{}).Collect(time.Now()); err == nil {
		t.Fatal("expected error when no system facts are available")
	}
}
