// Package power requests the performance CPU governor during STT/TTS bursts and
// restores the prior governor afterward. The wake-word detector itself runs at
// the device's default governor; only bursts ask for performance (Phase 2 spec
// P2.7). On a desktop / manual launch the sysfs nodes are not writable, so
// Request/Restore degrade to logged no-ops.
package power

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Governor switches all CPUs to a desired scaling governor and restores the
// originals. Request/Restore are refcounted so overlapping bursts nest safely.
type Governor struct {
	Root    string               // default "/sys/devices/system/cpu"
	Desired string               // default "performance"
	Logf    func(string, ...any) // optional warn logger

	mu    sync.Mutex
	depth int
	prev  map[string]string // path -> original governor
}

func (g *Governor) root() string {
	if g.Root == "" {
		return "/sys/devices/system/cpu"
	}
	return g.Root
}

func (g *Governor) desired() string {
	if g.Desired == "" {
		return "performance"
	}
	return g.Desired
}

func (g *Governor) paths() []string {
	matches, _ := filepath.Glob(filepath.Join(g.root(), "cpu[0-9]*", "cpufreq", "scaling_governor"))
	sort.Strings(matches)
	return matches
}

// Request switches all CPUs to the desired governor, recording originals on the
// outermost call. Nested calls only bump the refcount.
func (g *Governor) Request() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.depth++
	if g.depth > 1 {
		return nil
	}
	g.prev = map[string]string{}
	for _, p := range g.paths() {
		cur, err := os.ReadFile(p)
		if err != nil {
			g.warn("governor: read %s: %v", p, err)
			continue
		}
		g.prev[p] = strings.TrimSpace(string(cur))
		if err := os.WriteFile(p, []byte(g.desired()), 0o644); err != nil {
			g.warn("governor: set %s: %v", p, err)
		}
	}
	return nil
}

// Restore reverts to the recorded governors when the outermost burst ends.
func (g *Governor) Restore() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.depth == 0 {
		return nil
	}
	g.depth--
	if g.depth > 0 {
		return nil
	}
	for p, v := range g.prev {
		if err := os.WriteFile(p, []byte(v), 0o644); err != nil {
			g.warn("governor: restore %s: %v", p, err)
		}
	}
	g.prev = nil
	return nil
}

func (g *Governor) warn(format string, a ...any) {
	if g.Logf != nil {
		g.Logf(format, a...)
	}
}
