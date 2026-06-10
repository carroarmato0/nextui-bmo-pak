package renderer

import "testing"

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
		expr string
		want mouthKind
	}{
		{expr: "neutral", want: mouthNeutral},
		{expr: "idle", want: mouthNeutral},
		{expr: "speaking", want: mouthOpen},
		{expr: "sleeping", want: mouthSmile},
		{expr: "sleep", want: mouthSmile},
		{expr: "whistle", want: mouthWhistle},
		{expr: "concerned", want: mouthFrown},
		{expr: "error", want: mouthFrown},
		{expr: "confused", want: mouthFrown},
		{expr: "happy", want: mouthSmile},
		{expr: "excited", want: mouthOpen},
	}

	for _, tt := range tests {
		got := styleForExpression(tt.expr)
		if got.Mouth != tt.want {
			t.Fatalf("expression %q mapped to mouth %v, want %v", tt.expr, got.Mouth, tt.want)
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
