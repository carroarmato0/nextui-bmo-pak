package renderer

import (
	"testing"
)

func TestLayoutForScalesAcrossScreens(t *testing.T) {
	compact := LayoutFor(640, 480)
	wide := LayoutFor(1280, 720)

	if compact.W != 640 || compact.H != 480 {
		t.Fatalf("unexpected compact size: %+v", compact)
	}
	if wide.W != 1280 || wide.H != 720 {
		t.Fatalf("unexpected wide size: %+v", wide)
	}
	if compact.EyeW <= 0 || compact.EyeH <= 0 || compact.MouthW <= 0 {
		t.Fatalf("compact layout has invalid geometry: %+v", compact)
	}
	if wide.EyeW <= compact.EyeW {
		t.Fatalf("expected wider screen to allocate wider eyes: compact=%d wide=%d", compact.EyeW, wide.EyeW)
	}
	if wide.MouthW <= compact.MouthW {
		t.Fatalf("expected wider screen to allocate wider mouth: compact=%d wide=%d", compact.MouthW, wide.MouthW)
	}
}

func TestOverlayWindowClampsAndKeepsFocusVisible(t *testing.T) {
	// 10 visible rows, window of 4. Focus near the end must scroll.
	off := overlayWindow(10, 4, 9)
	if off < 0 || off > 6 {
		t.Fatalf("offset %d out of clamp range [0,6]", off)
	}
	if !(9 >= off && 9 < off+4) {
		t.Fatalf("focus 9 not within window [%d,%d)", off, off+4)
	}

	if off := overlayWindow(10, 4, 0); off != 0 {
		t.Fatalf("offset for focus 0 = %d, want 0", off)
	}

	if off := overlayWindow(3, 4, 2); off != 0 {
		t.Fatalf("offset when all fit = %d, want 0", off)
	}

	off = overlayWindow(10, 4, 8)
	if off+4 > 10 {
		t.Fatalf("window [%d,%d) overruns 10 rows", off, off+4)
	}
}

func TestOverlayWindowDegenerate(t *testing.T) {
	if off := overlayWindow(0, 4, 0); off != 0 {
		t.Fatalf("empty content offset = %d, want 0", off)
	}
	if off := overlayWindow(5, 0, 3); off != 0 {
		t.Fatalf("zero window offset = %d, want 0", off)
	}
}

func TestFramebufferEqual(t *testing.T) {
	a := []uint32{1, 2, 3}
	if !framebufferEqual(a, []uint32{1, 2, 3}) {
		t.Error("identical buffers should be equal")
	}
	if framebufferEqual(a, []uint32{1, 2, 4}) {
		t.Error("differing buffers should be unequal")
	}
	if framebufferEqual(a, []uint32{1, 2}) {
		t.Error("different-length buffers should be unequal")
	}
	if !framebufferEqual(nil, []uint32{}) {
		t.Error("empty buffers should be equal")
	}
}

// A freshly changed frame must be presented swapChainDepth times so every
// buffer in a multi-buffered swap chain holds it, then presentation stops.
func TestShouldPresentSettlesAfterStaticFrames(t *testing.T) {
	r := &Renderer{pixels: []uint32{0xAA, 0xBB, 0xCC}}
	for i := 0; i < swapChainDepth; i++ {
		if !r.shouldPresent() {
			t.Fatalf("present %d: want true while filling the swap chain", i)
		}
	}
	if r.shouldPresent() {
		t.Fatal("want false once every swap-chain buffer holds the frame")
	}
	if r.shouldPresent() {
		t.Fatal("want false to stay settled while the face is static")
	}
}

func TestShouldPresentRetriggersOnChange(t *testing.T) {
	r := &Renderer{pixels: []uint32{1, 1, 1}}
	for i := 0; i < swapChainDepth+2; i++ {
		r.shouldPresent() // settle
	}
	r.pixels = []uint32{1, 9, 1} // a single pixel changes
	for i := 0; i < swapChainDepth; i++ {
		if !r.shouldPresent() {
			t.Fatalf("post-change present %d: want true", i)
		}
	}
	if r.shouldPresent() {
		t.Fatal("want false after the change settles")
	}
}

// An arbitrary mod animation that changes pixels every frame must present every
// frame — no regression versus always-present.
func TestShouldPresentEveryFrameWhileAnimating(t *testing.T) {
	r := &Renderer{}
	for i := 0; i < 10; i++ {
		r.pixels = []uint32{uint32(i)}
		if !r.shouldPresent() {
			t.Fatalf("frame %d: want true while content changes every frame", i)
		}
	}
}

func TestShouldPresentResizeCountsAsChange(t *testing.T) {
	r := &Renderer{pixels: []uint32{1, 2, 3}}
	for i := 0; i < swapChainDepth+1; i++ {
		r.shouldPresent() // settle
	}
	r.pixels = []uint32{1, 2, 3, 4} // resize
	if !r.shouldPresent() {
		t.Fatal("a resize must count as a change and present")
	}
	if len(r.lastRendered) != len(r.pixels) {
		t.Fatalf("lastRendered len = %d, want %d", len(r.lastRendered), len(r.pixels))
	}
}
