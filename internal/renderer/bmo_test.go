package renderer

import (
	"testing"

	"github.com/carroarmato0/nextui-bmo/internal/face"
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

func TestBlitStripBounds(t *testing.T) {
	r := &Renderer{W: 8, H: 8, stride: 8, pixels: make([]uint32, 64)}
	strip := &face.Strip{X: 2, Y: 3, W: 4, H: 2, Pix: make([]uint32, 8)}
	for i := range strip.Pix {
		strip.Pix[i] = 0xFFFFFFFF
	}
	r.blitStrip(strip)
	if r.pixels[3*8+2] != 0xFFFFFFFF || r.pixels[4*8+5] != 0xFFFFFFFF {
		t.Fatal("strip pixels not blitted")
	}
	if r.pixels[0] != 0 || r.pixels[3*8+1] != 0 {
		t.Fatal("pixels outside strip must be untouched")
	}
	// Out-of-bounds strip must be silently ignored.
	r.blitStrip(&face.Strip{X: 7, Y: 7, W: 4, H: 4, Pix: make([]uint32, 16)})
}
