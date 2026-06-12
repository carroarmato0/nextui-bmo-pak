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

func TestStyleForExpression(t *testing.T) {
	tests := []struct {
		expr      string
		wantMouth bmoMouthType
	}{
		{expr: "neutral", wantMouth: bmoMouthIdleSmile},
		{expr: "idle", wantMouth: bmoMouthIdleSmile},
		{expr: "speaking", wantMouth: bmoMouthOpenSpeak},
		{expr: "sleeping", wantMouth: bmoMouthIdleSmile},
		{expr: "concerned", wantMouth: bmoMouthFrown},
		{expr: "error", wantMouth: bmoMouthFrown},
		{expr: "happy", wantMouth: bmoMouthOpenLarge},
		{expr: "excited", wantMouth: bmoMouthOpenLarge},
		{expr: "listening", wantMouth: bmoMouthOpenSmall},
	}
	for _, tt := range tests {
		got := styleForExpression(tt.expr)
		if got.Mouth != tt.wantMouth {
			t.Fatalf("expression %q: mouth %v, want %v", tt.expr, got.Mouth, tt.wantMouth)
		}
	}
}

func TestTongueAnchoredBelowMouthOpening(t *testing.T) {
	// Spec §6.2: the tongue is a dome rising from behind the lower lip — its
	// ellipse root sits at/below the bottom edge of the opening, its visible
	// top is inside the interior below the teeth band. Rendering clips it to
	// the opening so it never overlaps the lower lip.
	sizes := []struct {
		name   string
		mw, mh int32
	}{
		{name: "open-large", mw: 84, mh: 43},
		{name: "speak-base", mw: 64, mh: 33},
		{name: "speak-low-amplitude", mw: 64, mh: 4},
	}
	for _, tt := range sizes {
		mty := int32(100)
		ty, _, th := tongueGeometry(mty, tt.mw, tt.mh)
		mouthBottom := mty + tt.mh
		if ty+th < mouthBottom {
			t.Fatalf("%s: tongue root %d must reach the mouth bottom %d so it reads as coming from below", tt.name, ty+th, mouthBottom)
		}
		if ty >= mouthBottom {
			t.Fatalf("%s: tongue top %d is fully below the mouth (bottom %d) — nothing visible", tt.name, ty, mouthBottom)
		}
		teethBottom := mty + int32(float64(tt.mh)*0.28)
		if ty < teethBottom {
			t.Fatalf("%s: tongue top %d overlaps the teeth band ending at %d", tt.name, ty, teethBottom)
		}
	}
}

func TestNormalizeExpressionAliases(t *testing.T) {
	tests := []struct {
		expr string
		want string
	}{
		{expr: "idle", want: "neutral"},
		{expr: "neutral", want: "neutral"},
		{expr: "asleep", want: "sleeping"},
		{expr: "sleep", want: "sleeping"},
		{expr: "error", want: "concerned"},
		{expr: "confused", want: "concerned"},
		{expr: "angry", want: "concerned"},
		{expr: "sad", want: "concerned"},
		{expr: "happy", want: "smile"},
		{expr: "excited", want: "laugh"},
	}

	for _, tt := range tests {
		if got := normalizeExpression(tt.expr); got != tt.want {
			t.Fatalf("normalizeExpression(%q) = %q, want %q", tt.expr, got, tt.want)
		}
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
