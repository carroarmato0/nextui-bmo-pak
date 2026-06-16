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
