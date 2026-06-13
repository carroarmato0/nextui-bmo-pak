package face

import (
	"os"
	"path/filepath"
	"testing"
)

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

func TestCacheSpeakAnimated(t *testing.T) {
	c := NewCache(NewLibrary(""))
	base, strip := c.Speak(0, 320, 240)
	if base == nil || strip != nil {
		t.Fatal("level 0 must be the base frame with no strip")
	}
	base2, strip2 := c.Speak(1, 320, 240)
	if &base[0] != &base2[0] || strip2 == nil {
		t.Fatal("level >0 must reuse base and provide a strip")
	}
	if strip2.W <= 0 || strip2.H <= 0 || len(strip2.Pix) != strip2.W*strip2.H {
		t.Fatalf("malformed strip: %+v", strip2)
	}
	if strip2.X+strip2.W > 320 || strip2.Y+strip2.H > 240 {
		t.Fatalf("strip out of bounds: %+v", strip2)
	}
}

func TestCacheSpeakStaticModFile(t *testing.T) {
	dir := t.TempDir()
	static := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210"><rect x="0" y="0" width="280" height="210" fill="#4ECBA8"/><rect x="22" y="20" width="202" height="155" rx="14" fill="#90e5c8"/></svg>`
	if err := os.WriteFile(filepath.Join(dir, "speaking.svg"), []byte(static), 0o644); err != nil {
		t.Fatal(err)
	}
	c := NewCache(NewLibrary(dir))
	base, strip := c.Speak(0.7, 320, 240)
	if base == nil || strip != nil {
		t.Fatal("static mod speaking.svg must render as a static frame (no strip)")
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
