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
