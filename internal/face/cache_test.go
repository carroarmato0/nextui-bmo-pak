package face

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestCacheFrameRendersTemplatedNeutral(t *testing.T) {
	lib := NewLibrary(t.TempDir()) // embedded defaults only
	c := NewCache(lib)
	buf := c.Frame(ExprNeutral, 80, 60)
	if buf == nil {
		t.Fatal("neutral frame nil — templated face failed to rasterize at rest")
	}
	if len(buf) != 80*60 {
		t.Fatalf("frame size=%d want %d", len(buf), 80*60)
	}
}

func TestCacheFrameReturnsBuffer(t *testing.T) {
	c := NewCache(NewLibrary(""))
	buf := c.Frame(ExprNeutral, 320, 240)
	if len(buf) != 320*240 {
		t.Fatalf("expected %d pixels, got %d", 320*240, len(buf))
	}
}

func TestCacheFrameResizeInvalidates(t *testing.T) {
	c := NewCache(NewLibrary(""))
	buf1 := c.Frame(ExprNeutral, 320, 240)
	buf2 := c.Frame(ExprNeutral, 640, 480)
	if &buf1[0] == &buf2[0] {
		t.Fatal("resize must invalidate cache")
	}
	if len(buf2) != 640*480 {
		t.Fatalf("expected %d pixels after resize, got %d", 640*480, len(buf2))
	}
}

func TestCacheFrameAliasReuse(t *testing.T) {
	c := NewCache(NewLibrary(""))
	a := c.Frame("idle", 320, 240)
	b := c.Frame(ExprNeutral, 320, 240)
	if a == nil || b == nil {
		t.Fatal("both alias and canonical must return non-nil")
	}
	if &a[0] != &b[0] {
		t.Fatal("alias and canonical must share the same cached buffer")
	}
}

func TestCacheCorruptOverrideFallsBack(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "neutral.svg"), []byte("<svg garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := NewCache(NewLibrary(dir))
	buf := c.Frame(ExprNeutral, 320, 240)
	if buf == nil {
		t.Fatal("corrupt override must fall back to embedded default, not nil")
	}
	if len(buf) != 320*240 {
		t.Fatalf("expected %d pixels, got %d", 320*240, len(buf))
	}
	// The embedded neutral has dark eyes — confirm we got the real fallback, not zeros.
	assertColor(t, buf, 320, 240, 80, 78, 0x1a, 0x1a, 0x1a, "fallback neutral eye")
}

func TestFrameRendersCustomName(t *testing.T) {
	dir := t.TempDir()
	neutral := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210"><rect width="280" height="210" fill="#0000ff"/></svg>`
	grumpy := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210"><rect width="280" height="210" fill="#00ff00"/></svg>`
	if err := os.WriteFile(filepath.Join(dir, "neutral.svg"), []byte(neutral), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "grumpy.svg"), []byte(grumpy), 0o644); err != nil {
		t.Fatal(err)
	}
	c := NewCache(NewLibraryMode(dir, true))

	g := c.Frame("grumpy", 28, 21)
	if g == nil {
		t.Fatal("custom expression grumpy did not render")
	}
	n := c.Frame("neutral", 28, 21)
	if n == nil {
		t.Fatal("neutral did not render")
	}
	same := true
	for i := range g {
		if g[i] != n[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("grumpy frame equals neutral frame; custom name was folded to neutral")
	}
}

func TestCacheSourceDelegates(t *testing.T) {
	dir := t.TempDir()
	custom := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210"><rect width="280" height="210" fill="#321"/></svg>`
	if err := os.WriteFile(filepath.Join(dir, "grumpy.svg"), []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	c := NewCache(NewLibraryMode(dir, true))
	if got := c.Source("grumpy"); got != "mod-override" {
		t.Fatalf("Source = %q, want mod-override", got)
	}
	embedded := NewCache(NewLibrary(""))
	if got := embedded.Source(ExprNeutral); got != "embedded-default" {
		t.Fatalf("Source = %q, want embedded-default", got)
	}
}

func TestCacheEvictsUnpinnedModFacesBeyondBudget(t *testing.T) {
	c := &Cache{frames: map[string][]uint32{}, failed: map[string]bool{}}
	for i := 0; i < staticCacheModBudget+3; i++ {
		k := fmt.Sprintf("modface%d", i)
		c.frames[k] = []uint32{uint32(i)}
		c.noteUseLocked(k)
		c.evictLocked()
	}
	if len(c.lru) != staticCacheModBudget {
		t.Fatalf("unpinned resident = %d, want %d", len(c.lru), staticCacheModBudget)
	}
	if _, ok := c.frames["modface0"]; ok {
		t.Error("oldest mod face should have been evicted")
	}
	newest := fmt.Sprintf("modface%d", staticCacheModBudget+2)
	if _, ok := c.frames[newest]; !ok {
		t.Errorf("newest mod face %q must stay resident", newest)
	}
}

// Canonical faces are pinned: a mod with arbitrarily many custom faces must
// never evict the built-in expression set (the idle rotation).
func TestCachePinsCanonicalFacesAgainstFloodingMod(t *testing.T) {
	c := &Cache{frames: map[string][]uint32{}, failed: map[string]bool{}}
	for _, n := range CanonicalNames {
		c.frames[n] = []uint32{1}
		c.noteUseLocked(n)
	}
	for i := 0; i < staticCacheModBudget+100; i++ {
		k := fmt.Sprintf("modface%d", i)
		c.frames[k] = []uint32{2}
		c.noteUseLocked(k)
		c.evictLocked()
	}
	for _, n := range CanonicalNames {
		if _, ok := c.frames[n]; !ok {
			t.Fatalf("canonical %q was evicted; canonical faces must be pinned", n)
		}
	}
	if len(c.lru) != staticCacheModBudget {
		t.Errorf("unpinned resident = %d, want %d", len(c.lru), staticCacheModBudget)
	}
}

func TestCacheNoteUseUpdatesRecency(t *testing.T) {
	c := &Cache{frames: map[string][]uint32{}, failed: map[string]bool{}}
	for i := 0; i < staticCacheModBudget; i++ {
		k := fmt.Sprintf("m%d", i)
		c.frames[k] = []uint32{}
		c.noteUseLocked(k)
	}
	c.noteUseLocked("m0") // re-touch oldest → now most recent
	c.frames["new"] = []uint32{}
	c.noteUseLocked("new")
	c.evictLocked()
	if _, ok := c.frames["m0"]; !ok {
		t.Error("re-touched m0 must survive eviction")
	}
	if _, ok := c.frames["m1"]; ok {
		t.Error("now-oldest m1 should be evicted")
	}
}
