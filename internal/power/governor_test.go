package power

import (
	"os"
	"path/filepath"
	"testing"
)

func writeGov(t *testing.T, root, cpu, val string) string {
	t.Helper()
	dir := filepath.Join(root, cpu, "cpufreq")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "scaling_governor")
	if err := os.WriteFile(p, []byte(val+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestGovernorRequestAndRestore(t *testing.T) {
	root := t.TempDir()
	p0 := writeGov(t, root, "cpu0", "schedutil")
	p1 := writeGov(t, root, "cpu1", "schedutil")
	g := &Governor{Root: root, Desired: "performance"}

	if err := g.Request(); err != nil {
		t.Fatalf("request: %v", err)
	}
	for _, p := range []string{p0, p1} {
		if got := read(t, p); got != "performance" {
			t.Fatalf("%s = %q want performance", p, got)
		}
	}
	if err := g.Restore(); err != nil {
		t.Fatalf("restore: %v", err)
	}
	for _, p := range []string{p0, p1} {
		if got := read(t, p); got != "schedutil" {
			t.Fatalf("%s = %q want restored schedutil", p, got)
		}
	}
}

func TestGovernorRequestRefcounts(t *testing.T) {
	root := t.TempDir()
	p0 := writeGov(t, root, "cpu0", "schedutil")
	g := &Governor{Root: root, Desired: "performance"}

	_ = g.Request()
	_ = g.Request() // nested burst
	_ = g.Restore() // still in a burst
	if got := read(t, p0); got != "performance" {
		t.Fatalf("released too early: %q", got)
	}
	_ = g.Restore()
	if got := read(t, p0); got != "schedutil" {
		t.Fatalf("not restored after final release: %q", got)
	}
}

func TestGovernorRestoreWithoutRequestIsNoop(t *testing.T) {
	g := &Governor{Root: t.TempDir(), Desired: "performance"}
	if err := g.Restore(); err != nil {
		t.Fatalf("restore with no outstanding request should be a no-op, got %v", err)
	}
}

func TestGovernorMissingSysfsDoesNotError(t *testing.T) {
	// No cpufreq nodes under root: Request/Restore must not error (desktop).
	g := &Governor{Root: filepath.Join(t.TempDir(), "nonexistent"), Desired: "performance"}
	if err := g.Request(); err != nil {
		t.Fatalf("request on missing sysfs: %v", err)
	}
	if err := g.Restore(); err != nil {
		t.Fatalf("restore on missing sysfs: %v", err)
	}
}
